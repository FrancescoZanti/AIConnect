package auth

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/fzanti/aiconnect/internal/config"
	"github.com/go-ldap/ldap/v3"
	"github.com/sirupsen/logrus"
)

// isPublicPath controlla se il path richiesto è nella lista dei path pubblici
func isPublicPath(path string, publicPaths []string) bool {
	for _, publicPath := range publicPaths {
		// Supporta pattern con wildcard finale (es. "/ollama/*")
		if strings.HasSuffix(publicPath, "/*") {
			// Rimuove "/*" per ottenere il prefisso base (es. "/ollama/")
			prefix := strings.TrimSuffix(publicPath, "*")
			// Il path deve iniziare esattamente con il prefisso (incluso il /)
			// Questo garantisce che "/ollama/*" NON matchi "/ollama-admin/"
			if strings.HasPrefix(path, prefix) {
				return true
			}
		} else if strings.HasSuffix(publicPath, "/") {
			// Path che termina con "/" matcha tutti i subpath
			// Deve matchare esattamente il prefisso incluso il /
			// Questo garantisce che "/api/" NON matchi "/api-admin/"
			if strings.HasPrefix(path, publicPath) {
				return true
			}
		} else if path == publicPath {
			// Match esatto
			return true
		}
	}
	return false
}

// LDAPAuthMiddleware gestisce l'autenticazione LDAP e l'autorizzazione basata su gruppi AD
func LDAPAuthMiddleware(cfg *config.Config, log *logrus.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Se l'autenticazione AD non è abilitata, passa direttamente
			if cfg.AD.Enabled != nil && !*cfg.AD.Enabled {
				log.WithField("path", r.URL.Path).Debug("Autenticazione AD disabilitata, accesso consentito")
				next.ServeHTTP(w, r)
				return
			}

			// Controlla se il path è pubblico (accessibile senza autenticazione)
			if isPublicPath(r.URL.Path, cfg.AD.PublicPaths) {
				log.WithField("path", r.URL.Path).Debug("Path pubblico, accesso senza autenticazione")
				next.ServeHTTP(w, r)
				return
			}

			// Estrai credenziali dall'header Authorization
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				log.Warn("Richiesta senza header Authorization")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Verifica che sia Basic Auth
			if !strings.HasPrefix(authHeader, "Basic ") {
				log.Warn("Tipo autenticazione non supportato")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Decodifica credenziali Base64
			encoded := strings.TrimPrefix(authHeader, "Basic ")
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				log.WithError(err).Warn("Errore decodifica credenziali")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Separa username e password
			credentials := strings.SplitN(string(decoded), ":", 2)
			if len(credentials) != 2 {
				log.Warn("Formato credenziali invalido")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			username := credentials[0]
			password := credentials[1]

			// Autentica contro AD e verifica gruppi
			if err := authenticateAndAuthorize(cfg, log, username, password); err != nil {
				log.WithFields(logrus.Fields{
					"username": username,
					"error":    err.Error(),
				}).Warn("Autenticazione o autorizzazione fallita")
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			// Aggiungi username al contesto per audit
			r.Header.Set("X-Forwarded-User", username)

			log.WithField("username", username).Info("Autenticazione e autorizzazione riuscita")

			// Passa al prossimo handler
			next.ServeHTTP(w, r)
		})
	}
}

// authenticateAndAuthorize esegue bind LDAP e verifica appartenenza a gruppi autorizzati
func authenticateAndAuthorize(cfg *config.Config, log *logrus.Logger, username, password string) error {
	// Connessione al server LDAP
	l, err := ldap.DialURL(cfg.AD.LDAPURL)
	if err != nil {
		return fmt.Errorf("errore connessione LDAP: %w", err)
	}
	defer l.Close()

	// Bind con account di servizio per cercare l'utente
	if err := l.Bind(cfg.AD.BindDN, cfg.AD.BindPassword); err != nil {
		return fmt.Errorf("errore bind service account: %w", err)
	}

	// Cerca DN dell'utente
	searchRequest := ldap.NewSearchRequest(
		cfg.AD.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		fmt.Sprintf("(sAMAccountName=%s)", ldap.EscapeFilter(username)),
		[]string{"dn", "memberOf"},
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return fmt.Errorf("errore ricerca utente: %w", err)
	}

	if len(sr.Entries) == 0 {
		return fmt.Errorf("utente non trovato: %s", username)
	}

	userDN := sr.Entries[0].DN
	userGroups := sr.Entries[0].GetAttributeValues("memberOf")

	// Bind con credenziali utente per autenticazione
	if err := l.Bind(userDN, password); err != nil {
		return fmt.Errorf("credenziali invalide per utente %s", username)
	}

	// Verifica appartenenza a gruppi autorizzati
	authorized := false
	for _, allowedGroup := range cfg.AD.AllowedGroups {
		for _, userGroup := range userGroups {
			// Controllo case-insensitive del CN del gruppo
			if strings.Contains(strings.ToLower(userGroup), strings.ToLower(allowedGroup)) {
				authorized = true
				log.WithFields(logrus.Fields{
					"username":      username,
					"matched_group": allowedGroup,
				}).Debug("Utente autorizzato tramite gruppo")
				break
			}
		}
		if authorized {
			break
		}
	}

	if !authorized {
		return fmt.Errorf("utente %s non appartiene a nessun gruppo autorizzato", username)
	}

	return nil
}
