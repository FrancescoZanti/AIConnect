#!/bin/bash
set -e

# Script di installazione Ollama Metrics Server per Rocky Linux

echo "=== Installazione Ollama Metrics Server ==="

# Verifica root
if [[ $EUID -ne 0 ]]; then
   echo "Questo script deve essere eseguito come root" 
   exit 1
fi

# Colori
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

INSTALL_DIR="/opt/ollama-metrics"
SERVICE_PATH="/etc/systemd/system/ollama-metrics.service"

echo -e "${YELLOW}1. Installazione dipendenze Python${NC}"
dnf install -y python3 python3-pip
pip3 install psutil
echo -e "${GREEN}✓ Dipendenze installate${NC}"

echo -e "${YELLOW}2. Creazione directory installazione${NC}"
mkdir -p "$INSTALL_DIR"
echo -e "${GREEN}✓ Directory creata${NC}"

echo -e "${YELLOW}3. Copia script metriche${NC}"
if [ ! -f "./ollama_metrics.py" ]; then
    echo -e "${RED}✗ File ollama_metrics.py non trovato${NC}"
    exit 1
fi
cp ./ollama_metrics.py "$INSTALL_DIR/"
chmod +x "$INSTALL_DIR/ollama_metrics.py"
echo -e "${GREEN}✓ Script copiato${NC}"

echo -e "${YELLOW}4. Installazione service systemd${NC}"
if [ ! -f "./ollama-metrics.service" ]; then
    echo -e "${RED}✗ File service non trovato${NC}"
    exit 1
fi
cp ./ollama-metrics.service "$SERVICE_PATH"
systemctl daemon-reload
echo -e "${GREEN}✓ Service installato${NC}"

echo -e "${YELLOW}5. Abilitazione e avvio servizio${NC}"
systemctl enable ollama-metrics
systemctl start ollama-metrics
echo -e "${GREEN}✓ Servizio avviato${NC}"

echo ""
echo -e "${GREEN}=== Installazione completata ===${NC}"
echo ""
echo "Test endpoint:"
echo "  curl http://localhost:11434/metrics"
echo ""
echo "Controllo stato:"
echo "  systemctl status ollama-metrics"
echo ""
echo "Log:"
echo "  journalctl -u ollama-metrics -f"
