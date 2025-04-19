package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	// "github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/streadway/amqp"
)

type ItemPedido struct {
	ItemID     int `json:"item_id"`
	Quantidade int `json:"quantidade"`
}

type Pedido struct {
	ID    int          `json:"id"`
	UF    string       `json:"uf"`
	Itens []ItemPedido `json:"itens"`
}

type CDResposta struct {
	ItemID        int    `json:"item_id"`
	CDSelecionado string `json:"cd_selecionado"`
	Status        string `json:"status"`
}

type RespostaPedido struct {
	PedidoID int          `json:"pedido_id"`
	Itens    []CDResposta `json:"itens"`
}

// Métricas Prometheus
var (
	pedidoTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pedido_total",
			Help: "Número total de pedidos recebidos",
		},
		[]string{"status"}, // status do pedido (sucesso, erro)
	)

	pedidoLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pedido_latency_seconds",
			Help:    "Latência em segundos para processar um pedido",
			Buckets: prometheus.DefBuckets, // Intervalos padrão de latência
		},
		[]string{"status"}, // status de sucesso ou erro
	)

	pedidoErrorTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pedido_error_total",
			Help: "Número total de erros ao processar pedidos",
		},
		[]string{"error_type"}, // tipo de erro (por exemplo, timeout, erro na fila, etc.)
	)
)

func init() {
	// Registrar as métricas no Prometheus
	prometheus.MustRegister(pedidoTotal)
	prometheus.MustRegister(pedidoLatency)
	prometheus.MustRegister(pedidoErrorTotal)
}

func conectarRabbit() (*amqp.Connection, *amqp.Channel, error) {
	conn, err := amqp.Dial(os.Getenv("RABBITMQ_URL"))
	if err != nil {
		return nil, nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, err
	}
	return conn, ch, nil
}

func handlePedido(ch *amqp.Channel) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now() // Para medir a latência

		var pedido Pedido
		if err := json.NewDecoder(r.Body).Decode(&pedido); err != nil {
			// Contabiliza erro de JSON inválido
			pedidoErrorTotal.WithLabelValues("invalid_json").Inc()
			http.Error(w, "JSON inválido", http.StatusBadRequest)
			return
		}

		// Contabiliza pedido recebido com sucesso
		pedidoTotal.WithLabelValues("received").Inc()

		replyQueue, err := ch.QueueDeclare("", false, true, true, false, nil)
		if err != nil {
			// Contabiliza erro de fila de resposta
			pedidoErrorTotal.WithLabelValues("reply_queue_error").Inc()
			http.Error(w, "Erro fila de resposta", http.StatusInternalServerError)
			return
		}

		msgs, err := ch.Consume(replyQueue.Name, "", true, false, false, false, nil)
		if err != nil {
			// Contabiliza erro ao consumir a fila
			pedidoErrorTotal.WithLabelValues("consume_queue_error").Inc()
			http.Error(w, "Erro consumir fila resposta", http.StatusInternalServerError)
			return
		}

		correlationID := uuid.New().String()
		body, _ := json.Marshal(pedido)

		err = ch.Publish("", "novo_pedido", false, false, amqp.Publishing{
			ContentType:   "application/json",
			CorrelationId: correlationID,
			ReplyTo:       replyQueue.Name,
			Body:          body,
		})
		if err != nil {
			// Contabiliza erro ao enviar o pedido
			pedidoErrorTotal.WithLabelValues("publish_error").Inc()
			http.Error(w, "Erro ao enviar pedido", http.StatusInternalServerError)
			return
		}

		timeout := time.After(5 * time.Second)
		for {
			select {
			case msg := <-msgs:
				if msg.CorrelationId == correlationID {
					var resposta RespostaPedido
					_ = json.Unmarshal(msg.Body, &resposta)
					// Contabiliza pedido processado com sucesso
					pedidoTotal.WithLabelValues("success").Inc()
					pedidoLatency.WithLabelValues("success").Observe(time.Since(start).Seconds())
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(resposta)
					return
				}
			case <-timeout:
				// Contabiliza erro de timeout
				pedidoErrorTotal.WithLabelValues("timeout").Inc()
				http.Error(w, "Timeout esperando resposta do serviço", http.StatusGatewayTimeout)
				return
			}
		}
	}
}

func main() {
	_ = godotenv.Load()
	conn, ch, err := conectarRabbit()
	if err != nil {
		log.Fatalf("Erro ao conectar RabbitMQ: %v", err)
	}
	defer conn.Close()
	defer ch.Close()

	http.Handle("/metrics", promhttp.Handler())

	fmt.Println("🚀 ServicoPedido rodando em :8080")
	http.HandleFunc("/pedido", handlePedido(ch))
	log.Fatal(http.ListenAndServe(":8080", nil))
}
