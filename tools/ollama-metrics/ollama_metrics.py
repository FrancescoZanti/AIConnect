#!/usr/bin/env python3
"""
Ollama Metrics Server
Espone metriche CPU, RAM e GPU su endpoint HTTP per AIConnect load balancer
"""

import json
import psutil
import subprocess
import shutil
from http.server import HTTPServer, BaseHTTPRequestHandler
import logging

# Configurazione
PORT = 11434  # Stessa porta di Ollama, path diverso
METRICS_PATH = "/metrics"

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger('ollama-metrics')

# Verifica disponibilità nvidia-smi
NVIDIA_SMI_AVAILABLE = shutil.which('nvidia-smi') is not None
if NVIDIA_SMI_AVAILABLE:
    logger.info("GPU NVIDIA rilevata - monitoraggio GPU abilitato")
else:
    logger.info("GPU NVIDIA non rilevata - monitoraggio solo CPU/RAM")


def get_gpu_metrics():
    """Raccoglie metriche GPU NVIDIA tramite nvidia-smi"""
    if not NVIDIA_SMI_AVAILABLE:
        return []
    
    try:
        # Query nvidia-smi per metriche GPU
        cmd = [
            'nvidia-smi',
            '--query-gpu=index,name,utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw,power.limit',
            '--format=csv,noheader,nounits'
        ]
        
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=5)
        
        if result.returncode != 0:
            logger.error(f"nvidia-smi error: {result.stderr}")
            return []
        
        gpus = []
        for line in result.stdout.strip().split('\n'):
            if not line:
                continue
            
            parts = [p.strip() for p in line.split(',')]
            if len(parts) >= 8:
                gpu_index = int(parts[0])
                gpu_name = parts[1]
                gpu_util = float(parts[2]) if parts[2] != '[N/A]' else 0.0
                mem_used = float(parts[3]) if parts[3] != '[N/A]' else 0.0
                mem_total = float(parts[4]) if parts[4] != '[N/A]' else 1.0
                temp = float(parts[5]) if parts[5] != '[N/A]' else 0.0
                power_draw = float(parts[6]) if parts[6] != '[N/A]' else 0.0
                power_limit = float(parts[7]) if parts[7] != '[N/A]' else 1.0
                
                mem_percent = (mem_used / mem_total * 100) if mem_total > 0 else 0.0
                power_percent = (power_draw / power_limit * 100) if power_limit > 0 else 0.0
                
                gpus.append({
                    "index": gpu_index,
                    "name": gpu_name,
                    "utilization_percent": round(gpu_util, 2),
                    "memory_used_mb": round(mem_used, 2),
                    "memory_total_mb": round(mem_total, 2),
                    "memory_percent": round(mem_percent, 2),
                    "temperature_c": round(temp, 2),
                    "power_draw_w": round(power_draw, 2),
                    "power_limit_w": round(power_limit, 2),
                    "power_percent": round(power_percent, 2)
                })
        
        return gpus
    
    except subprocess.TimeoutExpired:
        logger.error("nvidia-smi timeout")
        return []
    except Exception as e:
        logger.error(f"Errore raccolta metriche GPU: {e}")
        return []


class MetricsHandler(BaseHTTPRequestHandler):
    """Handler per richieste metriche"""
    
    def do_GET(self):
        """Gestisce richieste GET"""
        if self.path == METRICS_PATH:
            self.handle_metrics()
        else:
            self.send_error(404, "Not Found")
    
    def handle_metrics(self):
        """Risponde con metriche CPU, RAM e GPU in formato JSON"""
        try:
            # Raccolta metriche CPU e RAM
            cpu_percent = psutil.cpu_percent(interval=1)
            memory = psutil.virtual_memory()
            ram_percent = memory.percent
            
            # Raccolta metriche GPU
            gpus = get_gpu_metrics()
            
            # Calcola carico medio GPU (se disponibili)
            gpu_avg_util = 0.0
            gpu_avg_mem = 0.0
            if gpus:
                gpu_avg_util = sum(g["utilization_percent"] for g in gpus) / len(gpus)
                gpu_avg_mem = sum(g["memory_percent"] for g in gpus) / len(gpus)
            
            metrics = {
                "cpu_percent": round(cpu_percent, 2),
                "ram_percent": round(ram_percent, 2),
                "gpu_count": len(gpus),
                "gpu_avg_utilization_percent": round(gpu_avg_util, 2),
                "gpu_avg_memory_percent": round(gpu_avg_mem, 2),
                "gpus": gpus,
                "timestamp": psutil.time.time()
            }
            
            # Risposta JSON
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps(metrics, indent=2).encode())
            
            logger.debug(f"Metriche inviate: CPU={cpu_percent}%, RAM={ram_percent}%, GPUs={len(gpus)}")
            
        except Exception as e:
            logger.error(f"Errore raccolta metriche: {e}")
            self.send_error(500, "Internal Server Error")
    
    def log_message(self, format, *args):
        """Override per logging personalizzato"""
        logger.info(f"{self.client_address[0]} - {format % args}")


def main():
    """Avvia server metriche"""
    server_address = ('', PORT)
    httpd = HTTPServer(server_address, MetricsHandler)
    
    logger.info(f"Server metriche avviato su porta {PORT}")
    logger.info(f"Endpoint: http://localhost:{PORT}{METRICS_PATH}")
    
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        logger.info("Shutdown server metriche")
        httpd.shutdown()


if __name__ == '__main__':
    main()
