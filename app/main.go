package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Response struct {
	Nome    string `json:"nome"`
	Horario string `json:"horario"`
}

var requestsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total de requisições recebidas",
	},
	[]string{"method", "endpoint", "status"},
)

func init() {
	prometheus.MustRegister(requestsTotal)
}

func korpHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		requestsTotal.WithLabelValues(r.Method, "/projeto-korp", "405").Inc()
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := Response{
		Nome:    "Projeto Korp",
		Horario: time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	// Tratamento explícito de falha na escrita do response
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		requestsTotal.WithLabelValues(r.Method, "/projeto-korp", "500").Inc()
		log.Printf("erro na serialização json: %v", err)
		return
	}
	requestsTotal.WithLabelValues(r.Method, "/projeto-korp", "200").Inc()
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/projeto-korp", korpHandler)
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// Inicia o servidor em uma goroutine
	go func() {
		log.Println("Servidor iniciado na porta 8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("falha crítica: %v", err)
		}
	}()

	// Intercepta sinais SIGINT e SIGTERM do SO (ex: docker stop)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Iniciando desligamento...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Erro no desligamento: %v", err)
	}
	log.Println("Servidor finalizado corretamente.")
}
