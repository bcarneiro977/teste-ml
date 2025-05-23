
# Fluxo de Pedido - Arquitetura

Este projeto simula um fluxo de pedido entre os serviços `API Gateway`, `servico-pedido`, `RabbitMQ`, `servico-consulta` e `Postgres`. A arquitetura foi projetada para demonstrar como diferentes componentes podem interagir em um cenário de microserviços.

## Arquitetura

O fluxo ocorre da seguinte forma:

1. **API Gateway (Nginx)**: O ponto de entrada para as requisições HTTP. Ele redireciona as requisições para o `servico-pedido` via proxy reverso.
2. **servico-pedido**: Recebe os pedidos, valida os dados e os publica em uma fila do RabbitMQ para processamento.
3. **RabbitMQ**: Fila onde os pedidos são armazenados e aguardam processamento pelo `servico-consulta`.
4. **servico-consulta**: Consome os pedidos da fila, realiza o processamento e gera uma resposta.
5. **Postgres**: Banco de dados utilizado por `servico-consulta` para armazenar informações relacionadas ao pedido, como os itens e a localização.

## Fluxo

### 1. Requisição de Pedido

O fluxo começa com o API Gateway recebendo uma requisição HTTP `POST` na rota `/servico-pedido/pedido`. O API Gateway (Nginx) encaminha esta requisição para o `servico-pedido`.

### 2. Processamento do Pedido (servico-pedido)

- O `servico-pedido` valida o JSON recebido e cria um pedido com informações como `ID`, `UF`, e os itens no pedido.
- Em seguida, o `servico-pedido` publica o pedido na fila `novo_pedido` do RabbitMQ, com a informação de `correlation_id` e a `reply_to` apontando para uma fila de resposta temporária.

### 3. Fila RabbitMQ

- O pedido é colocado na fila `novo_pedido` do RabbitMQ. O `servico-consulta` consome essa fila para processar o pedido.
  
### 4. Processamento do Pedido (servico-consulta)

- O `servico-consulta` consome a mensagem da fila e realiza o processamento necessário (ex: selecionando o centro de distribuição para cada item do pedido).
- O `servico-consulta` armazena os dados do pedido no banco de dados Postgres.
- Após o processamento, o `servico-consulta` envia uma resposta de volta para o `servico-pedido` através da fila de resposta temporária no RabbitMQ.

### 5. Resposta para o Cliente

- O `servico-pedido` escuta a fila de resposta e, ao receber a resposta, envia o resultado para o cliente via HTTP.
- O cliente recebe o status do pedido, incluindo os itens e o centro de distribuição selecionado para cada um.

---

## Tecnologias Usadas

- **API Gateway (Nginx)**: Responsável por redirecionar as requisições HTTP para os serviços.
- **servico-pedido**: Microserviço escrito em Go que lida com os pedidos e interage com RabbitMQ e o `servico-consulta`.
- **RabbitMQ**: Fila de mensagens usada para desacoplar os serviços e permitir a comunicação assíncrona entre `servico-pedido` e `servico-consulta`.
- **servico-consulta**: Microserviço escrito em Go que processa os pedidos da fila e grava as informações no Postgres.
- **Postgres**: Banco de dados relacional usado para armazenar os dados do pedido processados pelo `servico-consulta`.

---

## Como Rodar

1. **Clonar o repositório**:

    ```bash
    git clone https://github.com/bcarneiro977/teste-ml
    cd projeto
    ```

2. **Construir os containers**:

    Utilize o Docker Compose para construir e subir os containers:

    ```bash
    docker-compose up --build
    ```

    Isso iniciará os seguintes containers:
    - `nginx`: API Gateway
    - `servico-pedido`: Serviço responsável por receber pedidos e enviar para o RabbitMQ
    - `servico-consulta`: Serviço que consome os pedidos da fila e processa os dados
    - `postgres`: Banco de dados relacional onde as informações do pedido são armazenadas
    - `rabbitmq`: Fila de mensagens usada para comunicação assíncrona entre `servico-pedido` e `servico-consulta`

3. **Testar a API**:

    Após os containers estarem em funcionamento, você pode testar a API `POST /servico-pedido/pedido` no seguinte endpoint:

    ```http
    POST http://localhost/servico-pedido/pedido
    ```

    **Exemplo de corpo da requisição**:

    ```json
    {
      "id": 123,
      "uf": "SP",
      "itens": [
        {
          "item_id": 101,
          "quantidade": 2
        },
        {
          "item_id": 102,
          "quantidade": 1
        }
      ]
    }
    ```

4. **Monitorar o RabbitMQ**:

    O RabbitMQ tem uma interface de gerenciamento acessível em `http://localhost:15672`. As credenciais padrão são:
    - **Usuário**: `guest`
    - **Senha**: `guest`

    Você pode verificar as filas e mensagens ali.

---

## Endpoints

### 1. `/servico-pedido/pedido` (POST)

Recebe um pedido com os itens e envia para o RabbitMQ.

#### Exemplo de Corpo da Requisição:

```json
{
  "id": 123,
  "uf": "SP",
  "itens": [
    {
      "item_id": 101,
      "quantidade": 2
    },
    {
      "item_id": 102,
      "quantidade": 1
    }
  ]
}
```

#### Exemplo de Resposta:

```json
{
  "pedido_id": 123,
  "itens": [
    {
      "item_id": 101,
      "cd_selecionado": "CD1",
      "status": "OK"
    },
    {
      "item_id": 102,
      "cd_selecionado": "CD2",
      "status": "OK"
    }
  ]
}
```

---

## Variáveis de Ambiente

O projeto depende das seguintes variáveis de ambiente:

- `RABBITMQ_URL`: URL de conexão com o RabbitMQ (ex: `amqp://guest:guest@localhost:5672/`)
- `POSTGRES_URL`: URL de conexão com o banco de dados Postgres (ex: `postgres://user:password@localhost:5432/database`)

## Contribuição

Sinta-se à vontade para fazer contribuições! Basta criar um fork deste repositório, fazer suas modificações e enviar um pull request.

---

## Licença

Este projeto está licenciado sob a MIT License - veja o arquivo [LICENSE](LICENSE) para mais detalhes.
