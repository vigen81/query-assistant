# Query Assistant

An intelligent query assistant that uses ChatGPT to convert natural language prompts into ClickHouse SQL queries and execute them. Built with Go, Fiber, Swagger, and Uber FX.

## 🚀 Features

- **Natural Language to SQL**: Convert plain English questions into optimized ClickHouse queries
- **Schema-Aware**: Understands your database schema for accurate query generation
- **Secure Execution**: Query validation and timeout protection
- **RESTful API**: Clean API with Swagger documentation
- **Observable**: Structured logging with Graylog integration
- **Configurable**: AWS Parameter Store integration for different environments
- **High Performance**: Built with Fiber framework and proper dependency injection

## 📋 Prerequisites

- Go 1.24.4+
- Docker & Docker Compose
- ClickHouse
- OpenAI API Key
- (Optional) Graylog for centralized logging
- (Optional) AWS Account for Parameter Store

## 🛠 Installation

### Quick Start with Docker

1. **Clone the repository**
```bash
git clone <repository-url>
cd query-assistant
```

2. **Set environment variables**
```bash
export OPENAI_API_KEY=your-openai-api-key
export JWT_SECRET=your-secret-key
export SKIP_AUTH=true  # For testing without authentication
```

3. **Start the entire stack**
```bash
make docker-run
```

4. **Initialize sample ClickHouse tables**
```bash
make ch-init
```

5. **Check health**
```bash
make api-health
```

### Development Setup

1. **Install dependencies**
```bash
make deps
```

2. **Generate Swagger documentation**
```bash
make swagger
```

3. **Start infrastructure services**
```bash
docker-compose up -d clickhouse
```

4. **Start the application**
```bash
make run
```

## 📖 API Documentation

- **Swagger UI**: http://localhost:8080/swagger/
- **Health Check**: http://localhost:8080/health

## 🔧 Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `POD_ENV` | Environment (local/dev/prod) | `local` |
| `OPENAI_API_KEY` | OpenAI API key | Required |
| `CLICKHOUSE_HOST` | ClickHouse host | `localhost` |
| `CLICKHOUSE_PASSWORD` | ClickHouse password | Empty |
| `JWT_SECRET` | JWT signing secret | `your-secret-key` |
| `SKIP_AUTH` | Skip authentication | `false` |
| `GRAYLOG_ADDR` | Graylog address | `gelf-udp-service:12222` |

### Configuration Structure

```json
{
  "server": {
    "port": ":8080",
    "read_timeout": "30s",
    "write_timeout": "30s"
  },
  "clickhouse": {
    "host": "localhost",
    "port": "9000",
    "database": "default",
    "query_timeout": "60s"
  },
  "openai": {
    "api_key": "your-key",
    "model": "gpt-4-turbo-preview",
    "max_tokens": 2000,
    "temperature": 0.1
  },
  "query": {
    "max_execution_time": "30s",
    "max_result_rows": 10000,
    "forbidden_keywords": ["DROP", "DELETE", "TRUNCATE"]
  }
}
```

## 📡 API Usage

### Execute Natural Language Query

```bash
curl -X POST http://localhost:8080/api/v1/query/execute \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "prompt": "Show me the top 10 users by total purchase amount in the last 30 days",
    "site_id": 123,
    "timeout": 30
  }'
```

If you are running locally with `SKIP_AUTH=true`, you can omit the `Authorization` header.

**Response:**
```json
{
  "query_id": "550e8400-e29b-41d4-a716-446655440000",
  "prompt": "Show me the top 10 users by total purchase amount in the last 30 days",
  "generated_sql": "SELECT user_id, SUM(amount) as total_amount FROM purchases WHERE date >= today() - 30 GROUP BY user_id ORDER BY total_amount DESC LIMIT 10",
  "results": [
    {"user_id": 123, "total_amount": 5430.50},
    {"user_id": 456, "total_amount": 3210.75}
  ],
  "row_count": 10,
  "execution_time": 0.234,
  "timestamp": "2023-01-01T00:00:00Z"
}
```

### Get Database Schema

```bash
curl -X GET http://localhost:8080/api/v1/schema \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### Validate Query

```bash
curl -X POST http://localhost:8080/api/v1/query/validate \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "query": "SELECT * FROM users LIMIT 10",
    "site_id": 123
  }'
```

## 🏗 Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Frontend      │────▶│   Query API     │────▶│    OpenAI       │
└─────────────────┘     └────────┬────────┘     └─────────────────┘
                                 │                         │
                                 ▼                         ▼
                        ┌─────────────────┐     ┌─────────────────┐
                        │   ClickHouse    │     │  Schema Context │
                        └─────────────────┘     └─────────────────┘
```

### How It Works

1. **User submits a natural language prompt** via the API
2. **System retrieves database schema** from ClickHouse
3. **Prompt + Schema sent to ChatGPT** for query generation
4. **Generated query is validated** for security
5. **Query is executed** with timeout protection
6. **Results are returned** in JSON format

## 🚀 Development

### Available Commands

```bash
make help              # Show all available commands
make dev-setup         # Setup development environment
make swagger           # Generate API documentation
make test              # Run tests
make lint              # Run linter
make docker-run        # Start with Docker Compose
make logs              # View application logs
```

### Project Structure

```
query-assistant/
├── cmd/                    # Application entry points
│   └── server/            # Main server application
├── internal/              # Private application code
│   ├── clickhouse/        # ClickHouse client
│   ├── config/            # Configuration management
│   ├── handlers/          # HTTP handlers
│   ├── logger/            # Structured logging
│   ├── middleware/        # HTTP middleware
│   ├── models/            # Domain models
│   ├── openai/            # OpenAI client
│   ├── plugin/            # AWS Parameter Store plugin
│   ├── repository/        # Data access layer
│   ├── server/            # HTTP server
│   └── services/          # Business logic
├── docs/                  # API documentation
├── scripts/               # Utility scripts
└── Makefile              # Build commands
```

## 🔒 Security

- **Query Validation**: Prevents dangerous operations (DROP, DELETE, etc.)
- **Timeout Protection**: Configurable query execution timeouts
- **JWT Authentication**: Secure API access
- **Input Sanitization**: All inputs are validated
- **Rate Limiting**: Configurable rate limits (when implemented)

## 🔍 Monitoring & Logging

- **Health Checks**: `/health`, `/ready`, `/live` endpoints
- **Structured Logging**: JSON logs with correlation IDs
- **Graylog Integration**: Centralized log management
- **Request Tracing**: Track queries through the system

## 🧪 Testing

### Unit Tests
```bash
make test
```

### Integration Tests
```bash
make test-integration
```

### API Testing
```bash
# Test health
make api-health

# Test query execution
make api-query

# Test schema retrieval
make api-schema
```

## 🚀 Deployment

### AWS Parameter Store Setup

1. **Create configurations**
```bash
# Development
make aws-create-dev-config

# Production
make aws-create-prod-config
```

2. **Deploy to environment**
```bash
# Development
POD_ENV=dev make deploy-dev

# Production
POD_ENV=prod make deploy-prod
```

## 📚 Examples

### Common Queries

```json
{
  "prompt": "Show total sales by month for the last year"
}

{
  "prompt": "Find users who made purchases over $1000 in the last week"
}

{
  "prompt": "What are the top selling products today?"
}

{
  "prompt": "Show me user registration trends by day for the last month"
}
```

## 🤝 Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📝 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🆘 Troubleshooting

### ClickHouse Connection Issues
```bash
# Check ClickHouse status
docker-compose ps clickhouse

# View ClickHouse logs
docker-compose logs clickhouse

# Test connection
docker-compose exec clickhouse clickhouse-client --query "SELECT 1"
```

### OpenAI API Issues
- Verify your API key is correct
- Check API rate limits
- Ensure the model name is valid

### Query Generation Issues
- Check that your tables have proper schema
- Verify column names and types
- Review the generated SQL in logs

## 🎯 Roadmap

- [ ] Query result caching
- [ ] Support for more SQL operations
- [ ] Query history and analytics
- [ ] Multiple database support
- [ ] Query optimization suggestions
- [ ] Export results to various formats
- [ ] Real-time query collaboration

---

**Built with ❤️ using Go, Fiber, ClickHouse, and OpenAI**