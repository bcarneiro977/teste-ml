package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
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
		return nil, nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, err
	}

	_, err = ch.QueueDeclare(
		"novo_pedido",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	return conn, ch, nil
}

func conectarPostgres() (*sql.DB, error) {
	return sql.Open("postgres", os.Getenv("DATABASE_URL"))
}

func selecionarCDMaisProximo(db *sql.DB, itemID int, uf string, quantidade int) (string, string) {
	query := `
		SELECT dc.name, dc.uf, ci.quantity FROM cd_items ci
		JOIN distribution_centers dc ON dc.id = ci.cd_id
		WHERE ci.item_id = $1 AND ci.quantity >= $2
		ORDER BY CASE WHEN dc.uf = $3 THEN 0 ELSE 1 END, dc.id
		LIMIT 1`

	var name, cdUF string
	var qtd int
	err := db.QueryRow(query, itemID, quantidade, uf).Scan(&name, &cdUF, &qtd)
	if err != nil {
		return "", "Indisponível"
	}
	return name, "OK"
}

func main() {
	_ = godotenv.Load()

	db, err := conectarPostgres()
	if err != nil {
		log.Fatalf("Erro ao conectar Postgres: %v", err)
	}
	defer db.Close()

	rabbitConn, rabbitCh, err := conectarRabbit()
	if err != nil {
		log.Fatalf("Erro ao conectar RabbitMQ: %v", err)
	}
	defer rabbitConn.Close()
	defer rabbitCh.Close()

	msgs, err := rabbitCh.Consume("novo_pedido", "", true, false, false, false, nil)
	if err != nil {
		log.Fatalf("Erro ao consumir fila: %v", err)
	}

	log.Println("📦 Service2 ouvindo fila 'novo_pedido'...")

	for msg := range msgs {
		var pedido Pedido
		_ = json.Unmarshal(msg.Body, &pedido)

		var resposta RespostaPedido
		resposta.PedidoID = pedido.ID

		for _, item := range pedido.Itens {
			cd, status := selecionarCDMaisProximo(db, item.ItemID, pedido.UF, item.Quantidade)
			resposta.Itens = append(resposta.Itens, CDResposta{
				ItemID:        item.ItemID,
				CDSelecionado: cd,
				Status:        status,
			})
		}

		responseBody, _ := json.Marshal(resposta)
		err := rabbitCh.Publish("", msg.ReplyTo, false, false, amqp.Publishing{
			ContentType:   "application/json",
			CorrelationId: msg.CorrelationId,
			Body:          responseBody,
		})
		if err != nil {
			log.Printf("Erro ao enviar resposta: %v", err)
		}
	}
}
