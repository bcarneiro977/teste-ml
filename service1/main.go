package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
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

func conectarRabbit() (*amqp.Connection, *amqp.Channel, error) {
	conn, err := amqp.Dial(os.Getenv("RABBITMQ_URL"))
	if err != nil {
		log.Printf("Erro ao conectar ao RabbitMQ: %v", err)
		return nil, nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		log.Printf("Erro ao criar canal RabbitMQ: %v", err)
		return nil, nil, err
	}
	log.Println("Conexão com RabbitMQ estabelecida com sucesso")
	return conn, ch, nil
}

func handlePedido(ch *amqp.Channel) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var pedido Pedido
		if err := json.NewDecoder(r.Body).Decode(&pedido); err != nil {
			log.Printf("Erro ao decodificar o JSON da requisição: %v", err)
			http.Error(w, "JSON inválido", http.StatusBadRequest)
			return
		}
		log.Printf("Pedido recebido: %+v", pedido)

		_, err := ch.QueueDeclare(
			"novo_pedido",
			true,
			false,
			false,
			false,
			nil,
		)
		if err != nil {
			log.Printf("Erro ao declarar fila 'novo_pedido': %v", err)
			http.Error(w, "Erro ao declarar fila 'novo_pedido'", http.StatusInternalServerError)
			return
		}
		log.Println("Fila 'novo_pedido' declarada com sucesso")

		replyQueue, err := ch.QueueDeclare("", false, true, true, false, nil)
		if err != nil {
			log.Printf("Erro ao criar fila de resposta: %v", err)
			http.Error(w, "Erro ao criar fila de resposta", http.StatusInternalServerError)
			return
		}
		log.Printf("Fila de resposta temporária criada: %s", replyQueue.Name)

		msgs, err := ch.Consume(replyQueue.Name, "", true, false, false, false, nil)
		if err != nil {
			log.Printf("Erro ao consumir fila de resposta: %v", err)
			http.Error(w, "Erro ao consumir fila de resposta", http.StatusInternalServerError)
			return
		}

		correlationID := uuid.New().String()
		log.Printf("CorrelationID gerado: %s", correlationID)

		body, err := json.Marshal(pedido)
		if err != nil {
			log.Printf("Erro ao serializar pedido para JSON: %v", err)
			http.Error(w, "Erro ao processar pedido", http.StatusInternalServerError)
			return
		}

		err = ch.Publish(
			"",
			"novo_pedido",
			false,
			false,
			amqp.Publishing{
				ContentType:   "application/json",
				CorrelationId: correlationID,
				ReplyTo:       replyQueue.Name,
				Body:          body,
			},
		)
		if err != nil {
			log.Printf("Erro ao enviar pedido para fila: %v", err)
			http.Error(w, "Erro ao enviar pedido para fila", http.StatusInternalServerError)
			return
		}
		log.Println("Pedido enviado para fila 'novo_pedido' com sucesso")

		for msg := range msgs {

			log.Printf("Mensagem recebida, CorrelationId: %s, Resposta: %s", msg.CorrelationId, string(msg.Body))

			if msg.CorrelationId == correlationID {
				var resposta RespostaPedido
				if err := json.Unmarshal(msg.Body, &resposta); err != nil {
					log.Printf("Erro ao decodificar resposta: %v", err)
					http.Error(w, "Erro ao decodificar resposta", http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resposta)
				log.Printf("Resposta enviada ao cliente: %+v", resposta)
				break
			}
		}
	}
}

func main() {
	_ = godotenv.Load()

	conn, ch, err := conectarRabbit()
	if err != nil {
		log.Fatalf("Erro ao conectar ao RabbitMQ: %v", err)
	}
	defer conn.Close()
	defer ch.Close()

	http.HandleFunc("/pedido", handlePedido(ch))

	log.Println("🚀 Service1 rodando em :8080")
	fmt.Println("🚀 Service1 rodando em :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
