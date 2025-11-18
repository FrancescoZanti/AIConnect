#!/bin/bash
set -e

# Script di installazione AIConnect per Rocky Linux

echo "=== Installazione AIConnect ==="

# Verifica root
if [[ $EUID -ne 0 ]]; then
   echo "Questo script deve essere eseguito come root" 
   exit 1
fi

# Colori per output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Variabili
BINARY_PATH="/usr/local/bin/aiconnect"
SERVICE_PATH="/etc/systemd/system/aiconnect.service"
CONFIG_DIR="/etc/aiconnect"
CONFIG_PATH="$CONFIG_DIR/config.yaml"
CACHE_DIR="/var/cache/aiconnect/autocert"
USER="aiconnect"
GROUP="aiconnect"

echo -e "${YELLOW}1. Creazione utente e gruppo di sistema${NC}"
if ! id "$USER" &>/dev/null; then
    useradd -r -s /sbin/nologin -d /var/cache/aiconnect "$USER"
    echo -e "${GREEN}✓ Utente $USER creato${NC}"
else
    echo -e "${GREEN}✓ Utente $USER già esistente${NC}"
fi

echo -e "${YELLOW}2. Creazione directories${NC}"
mkdir -p "$CONFIG_DIR"
mkdir -p "$CACHE_DIR"
chown -R "$USER:$GROUP" "$CACHE_DIR"
chmod 700 "$CACHE_DIR"
echo -e "${GREEN}✓ Directory create${NC}"

echo -e "${YELLOW}3. Copia binario${NC}"
if [ ! -f "./aiconnect" ]; then
    echo -e "${RED}✗ Binario ./aiconnect non trovato. Eseguire prima 'go build'${NC}"
    exit 1
fi
cp ./aiconnect "$BINARY_PATH"
chmod +x "$BINARY_PATH"
echo -e "${GREEN}✓ Binario installato in $BINARY_PATH${NC}"

echo -e "${YELLOW}4. Copia service file${NC}"
if [ ! -f "./deployment/aiconnect.service" ]; then
    echo -e "${RED}✗ File service non trovato${NC}"
    exit 1
fi
cp ./deployment/aiconnect.service "$SERVICE_PATH"
echo -e "${GREEN}✓ Service file installato${NC}"

echo -e "${YELLOW}5. Configurazione${NC}"
if [ ! -f "$CONFIG_PATH" ]; then
    if [ -f "./config.example.yaml" ]; then
        cp ./config.example.yaml "$CONFIG_PATH"
        echo -e "${YELLOW}! File di configurazione di esempio copiato in $CONFIG_PATH${NC}"
        echo -e "${YELLOW}! IMPORTANTE: Modifica $CONFIG_PATH con i tuoi parametri prima di avviare il servizio${NC}"
    else
        echo -e "${RED}✗ Nessun file di configurazione trovato${NC}"
        exit 1
    fi
else
    echo -e "${GREEN}✓ File di configurazione già esistente${NC}"
fi
chmod 600 "$CONFIG_PATH"
chown root:root "$CONFIG_PATH"

echo -e "${YELLOW}6. Configurazione firewall${NC}"
if command -v firewall-cmd &> /dev/null; then
    firewall-cmd --permanent --add-service=http
    firewall-cmd --permanent --add-service=https
    firewall-cmd --permanent --add-port=9090/tcp  # Prometheus metrics
    firewall-cmd --reload
    echo -e "${GREEN}✓ Firewall configurato (HTTP, HTTPS, Metrics)${NC}"
else
    echo -e "${YELLOW}! firewall-cmd non trovato, configura manualmente il firewall${NC}"
fi

echo -e "${YELLOW}7. Abilitazione servizio systemd${NC}"
systemctl daemon-reload
systemctl enable aiconnect
echo -e "${GREEN}✓ Servizio abilitato${NC}"

echo ""
echo -e "${GREEN}=== Installazione completata ===${NC}"
echo ""
echo "Prossimi passi:"
echo "1. Modifica la configurazione: nano $CONFIG_PATH"
echo "2. Avvia il servizio: systemctl start aiconnect"
echo "3. Controlla lo stato: systemctl status aiconnect"
echo "4. Visualizza i log: journalctl -u aiconnect -f"
echo "5. Metriche Prometheus disponibili su: http://localhost:9090/metrics"
echo ""
echo -e "${YELLOW}ATTENZIONE: Verifica che la porta 80 sia accessibile per ACME challenge${NC}"
