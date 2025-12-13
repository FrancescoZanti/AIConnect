package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

type WizardOptions struct {
	ConfigPath string
	Force      bool
}

// RunWizard avvia un wizard interattivo per creare/aggiornare la configurazione.
// Scrive il file su opts.ConfigPath e ritorna la Config risultante.
func RunWizard(opts WizardOptions) (*Config, error) {
	if strings.TrimSpace(opts.ConfigPath) == "" {
		return nil, errors.New("config path vuoto")
	}

	if !isInteractiveStdin() {
		return nil, errors.New("wizard non interattivo: stdin non è un terminale")
	}

	if !opts.Force {
		if _, err := os.Stat(opts.ConfigPath); err == nil {
			ok, err := askYesNo("Il file config esiste già. Vuoi sovrascriverlo?", false)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, errors.New("operazione annullata")
			}
		}
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		cfg := &Config{}

		fmt.Fprintln(os.Stdout, "\n=== AIConnect: configurazione guidata ===")

		adEnabled, err := askYesNo("Abilitare autenticazione Active Directory (LDAP)?", true)
		if err != nil {
			return nil, err
		}
		cfg.AD.Enabled = &adEnabled

		if adEnabled {
			cfg.AD.LDAPURL, err = askString(reader, "LDAP URL", "ldap://ad.example.com:389", true)
			if err != nil {
				return nil, err
			}
			cfg.AD.BaseDN, err = askString(reader, "Base DN", "DC=example,DC=com", true)
			if err != nil {
				return nil, err
			}
			cfg.AD.BindDN, err = askString(reader, "Bind DN (account di servizio)", "CN=service-account,OU=ServiceAccounts,DC=example,DC=com", true)
			if err != nil {
				return nil, err
			}
			cfg.AD.BindPassword, err = askSecret("Bind password (account di servizio)", true)
			if err != nil {
				return nil, err
			}

			groups, err := askCSV(reader, "Allowed groups (DN) - separa con virgola", []string{"CN=AI-Users,OU=Groups,DC=example,DC=com"}, true)
			if err != nil {
				return nil, err
			}
			cfg.AD.AllowedGroups = groups

			publicPaths, err := askCSV(reader, "Public paths (opzionale) - separa con virgola", nil, false)
			if err != nil {
				return nil, err
			}
			cfg.AD.PublicPaths = publicPaths
		}

		cfg.Backends.OllamaServers, err = askCSV(reader, "Backend Ollama (URL) - separa con virgola", nil, false)
		if err != nil {
			return nil, err
		}

		cfg.Backends.VLLMServers, err = askCSV(reader, "Backend vLLM (URL) - separa con virgola", nil, false)
		if err != nil {
			return nil, err
		}

		useOpenAI, err := askYesNo("Abilitare backend OpenAI?", false)
		if err != nil {
			return nil, err
		}
		if useOpenAI {
			cfg.Backends.OpenAIEndpoint, err = askString(reader, "OpenAI endpoint", "https://api.openai.com/v1", true)
			if err != nil {
				return nil, err
			}
			cfg.Backends.OpenAIAPIKey, err = askSecret("OpenAI API key", true)
			if err != nil {
				return nil, err
			}
		}

		cfg.HTTPS.Domain, err = askString(reader, "HTTPS domain (LetsEncrypt)", "aiconnect.example.com", true)
		if err != nil {
			return nil, err
		}
		cfg.HTTPS.CacheDir, err = askString(reader, "Cache dir certificati", "/var/cache/aiconnect/autocert", true)
		if err != nil {
			return nil, err
		}
		cfg.HTTPS.Port, err = askInt(reader, "Porta HTTPS", 443)
		if err != nil {
			return nil, err
		}

		cfg.Monitoring.HealthCheckInterval, err = askInt(reader, "Intervallo health check (secondi)", 30)
		if err != nil {
			return nil, err
		}
		cfg.Monitoring.MetricsPort, err = askInt(reader, "Porta metriche Prometheus", 9090)
		if err != nil {
			return nil, err
		}

		cfg.Logging.Level, err = askString(reader, "Log level (debug/info/warn/error)", "info", true)
		if err != nil {
			return nil, err
		}
		cfg.Logging.Format, err = askString(reader, "Log format (json/text)", "json", true)
		if err != nil {
			return nil, err
		}

		cfg.MDNS.Enabled, err = askYesNo("Abilitare mDNS advertisement?", true)
		if err != nil {
			return nil, err
		}
		cfg.MDNS.DiscoveryEnabled, err = askYesNo("Abilitare mDNS discovery?", true)
		if err != nil {
			return nil, err
		}
		cfg.MDNS.ServiceName, err = askString(reader, "mDNS service name", "AIConnect Orchestrator", true)
		if err != nil {
			return nil, err
		}
		cfg.MDNS.Version, err = askString(reader, "mDNS version", "1.0.0", true)
		if err != nil {
			return nil, err
		}
		cfg.MDNS.Capabilities, err = askString(reader, "mDNS capabilities", "ollama,vllm,openai", true)
		if err != nil {
			return nil, err
		}
		cfg.MDNS.DiscoveryInterval, err = askInt(reader, "mDNS discovery interval (secondi)", 30)
		if err != nil {
			return nil, err
		}
		cfg.MDNS.DiscoveryTimeout, err = askInt(reader, "mDNS discovery timeout (secondi)", 5)
		if err != nil {
			return nil, err
		}
		cfg.MDNS.ServiceTypes, err = askCSV(reader, "mDNS service types - separa con virgola", []string{"_ollama._tcp", "_openai._tcp", "_vllm._tcp"}, true)
		if err != nil {
			return nil, err
		}

		if err := Validate(cfg); err != nil {
			fmt.Fprintf(os.Stdout, "\nConfig non valida: %v\n", err)
			ok, askErr := askYesNo("Vuoi riprovare il wizard?", true)
			if askErr != nil {
				return nil, askErr
			}
			if ok {
				continue
			}
			return nil, err
		}

		fmt.Fprintln(os.Stdout, "\nConfig valida.")
		ok, err := askYesNo("Salvare la configurazione?", true)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errors.New("operazione annullata")
		}

		if err := Save(opts.ConfigPath, cfg); err != nil {
			return nil, err
		}
		fmt.Fprintf(os.Stdout, "Configurazione salvata in: %s\n", opts.ConfigPath)
		return cfg, nil
	}
}

func askString(r *bufio.Reader, label, def string, required bool) (string, error) {
	prompt := label
	if def != "" {
		prompt = fmt.Sprintf("%s [%s]", label, def)
	}
	for {
		fmt.Fprintf(os.Stdout, "%s: ", prompt)
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			line = def
		}
		if required && strings.TrimSpace(line) == "" {
			fmt.Fprintln(os.Stdout, "Valore obbligatorio")
			continue
		}
		return line, nil
	}
}

func askInt(r *bufio.Reader, label string, def int) (int, error) {
	for {
		fmt.Fprintf(os.Stdout, "%s [%d]: ", label, def)
		line, err := r.ReadString('\n')
		if err != nil {
			return 0, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return def, nil
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			fmt.Fprintln(os.Stdout, "Numero non valido")
			continue
		}
		return n, nil
	}
}

func askCSV(r *bufio.Reader, label string, def []string, required bool) ([]string, error) {
	defStr := ""
	if len(def) > 0 {
		defStr = strings.Join(def, ",")
	}
	val, err := askString(r, label, defStr, required)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(val) == "" {
		return nil, nil
	}
	parts := strings.Split(val, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if required && len(out) == 0 {
		return nil, errors.New("lista vuota")
	}
	return out, nil
}

func askYesNo(label string, def bool) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	defStr := "n"
	if def {
		defStr = "y"
	}
	for {
		fmt.Fprintf(os.Stdout, "%s [y/n] (default %s): ", label, defStr)
		line, err := reader.ReadString('\n')
		if err != nil {
			return false, err
		}
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "" {
			return def, nil
		}
		switch line {
		case "y", "yes", "s", "si":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(os.Stdout, "Rispondi y/n")
		}
	}
}

func askSecret(label string, required bool) (string, error) {
	for {
		fmt.Fprintf(os.Stdout, "%s: ", label)

		fd := int(os.Stdin.Fd())
		oldState, err := unix.IoctlGetTermios(fd, unix.TCGETS)
		if err == nil {
			newState := *oldState
			newState.Lflag &^= unix.ECHO
			_ = unix.IoctlSetTermios(fd, unix.TCSETS, &newState)

			r := bufio.NewReader(os.Stdin)
			line, readErr := r.ReadString('\n')
			_ = unix.IoctlSetTermios(fd, unix.TCSETS, oldState)
			fmt.Fprintln(os.Stdout)

			if readErr != nil {
				return "", readErr
			}
			s := strings.TrimSpace(line)
			if required && s == "" {
				fmt.Fprintln(os.Stdout, "Valore obbligatorio")
				continue
			}
			return s, nil
		}

		// Fallback: se non riusciamo a modificare il terminale, leggiamo in chiaro.
		r := bufio.NewReader(os.Stdin)
		line, readErr := r.ReadString('\n')
		if readErr != nil {
			return "", readErr
		}
		s := strings.TrimSpace(line)
		if required && s == "" {
			fmt.Fprintln(os.Stdout, "Valore obbligatorio")
			continue
		}
		return s, nil
	}
}

func isInteractiveStdin() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
