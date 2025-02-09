package redis

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/ydb-platform/fq-connector-go/common"

	rdbms_utils "github.com/ydb-platform/fq-connector-go/app/server/datasource/rdbms/utils"
)

var _ rdbms_utils.Connection = (*Connection)(nil)

// Connection - реализация rdbms_utils.Connection для Redis.
type Connection struct {
	client *redis.Client
	logger common.QueryLogger
}

// Close закрывает соединение с Redis.
func (c *Connection) Close() error {
	return c.client.Close()
}

// From возвращает (databaseName, tableName), если нужно.
// В случае Redis обычно пусто, так как отдельной концепции "базы" / "таблицы" нет.
func (c *Connection) From() (databaseName, tableName string) {
	return "", "example_table"
}

// Query обрабатывает входящие SQL-подобные запросы (только SELECT),
// разбирает их парсером, переводит в команды Redis и выполняет.
func (c *Connection) Query(params *rdbms_utils.QueryParams) (rdbms_utils.Rows, error) {
	c.logger.Dump(params.QueryText, params.QueryArgs.Values()...)

	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(params.QueryText)), "describe ") {
		return c.handleDescribe(params.Ctx, params.QueryText)
	}

	// Парсим SQL-запрос (ограничимся SELECT)
	stmt, err := parseSelect(params.QueryText)
	if err != nil {
		c.logger.Dump("Failed to parse SQL query", err)
		return nil, err
	}

	// Преобразуем AST (stmt) в список Redis-команд
	cmds, err := translateToRedisCommands(stmt)
	if err != nil {
		c.logger.Dump("Failed to translate to Redis commands", err)
		return nil, err
	}

	// Выполним полученные команды
	results, err := executeRedisCommands(params.Ctx, c.client, cmds)
	if err != nil {
		c.logger.Dump("Failed to execute Redis commands", err)
		return nil, err
	}

	fmt.Println("RESULTS:", results)

	convertedResults, err := convertResults(results)
	if err != nil {
		c.logger.Dump("Failed to convert results", err)
		return nil, err
	}

	// Результаты "оборачиваем" в rdbms_utils.Rows (в примере - в простую структуру `rows`)
	return &rows{result: convertedResults}, nil
}

// SelectStatement описывает структуру разбранного SELECT-запроса.
type SelectStatement struct {
	Columns  []string // например, [field1, field2] или ["*"]
	Table    string   // псевдоним "таблицы" (в Redis - обычно это namespace / префикс)
	Key      string   // значение из WHERE key = '...'
	HasWhere bool     // был ли вообще WHERE
	Limit    int      // если задан LIMIT
	HasLimit bool     // был ли LIMIT
}

// parseSelect парсит строку SQL-подобного запроса (упрощённая форма).
// Поддерживаем: SELECT <cols> FROM <table> [WHERE key = '...'] [LIMIT N]
func parseSelect(sqlQuery string) (*SelectStatement, error) {
	stmt := &SelectStatement{Limit: -1}

	q := strings.TrimSpace(sqlQuery)
	q = strings.ToLower(q)
	fmt.Println(q)

	if !strings.HasPrefix(q, "select ") {
		return nil, errors.New("query must start with SELECT")
	}
	// Отбрасываем "select " (7 символов)
	q = strings.TrimSpace(q[6:])

	// Ищем " from " как границу между колонками и таблицей
	fromIdx := strings.Index(q, " from ")
	if fromIdx == -1 {
		return nil, errors.New("missing FROM clause")
	}

	columnsPart := strings.TrimSpace(q[:fromIdx])
	rest := strings.TrimSpace(q[fromIdx+6:]) // 6 = len(" from ")

	// Разбиваем список колонок (для простоты - по запятой)
	cols := strings.Split(columnsPart, ",")
	for i := range cols {
		cols[i] = strings.TrimSpace(cols[i])
	}
	stmt.Columns = cols

	// Теперь парсим остальную часть: <table> [where key = '...'] [limit n]
	tokens := strings.Fields(rest)
	if len(tokens) == 0 {
		return nil, errors.New("missing table name after FROM")
	}
	stmt.Table = tokens[0]
	tokens = tokens[1:]

	i := 0
	for i < len(tokens) {
		switch tokens[i] {
		case "where":
			// Ожидаем "where key = 'xxx'"
			if i+3 >= len(tokens) {
				return nil, errors.New("invalid WHERE syntax")
			}
			if tokens[i+1] != "key" || tokens[i+2] != "=" {
				return nil, fmt.Errorf("expected `where key = ...`, got: %s %s %s",
					tokens[i+1], tokens[i+2], tokens[i+3])
			}
			stmt.HasWhere = true
			stmt.Key = strings.Trim(tokens[i+3], "'") // убираем кавычки
			i += 4
		case "limit":
			// limit N
			if i+1 >= len(tokens) {
				return nil, errors.New("invalid LIMIT syntax")
			}
			limitVal, err := strconv.Atoi(tokens[i+1])
			if err != nil {
				return nil, fmt.Errorf("invalid LIMIT value: %s", tokens[i+1])
			}
			stmt.HasLimit = true
			stmt.Limit = limitVal
			i += 2
		default:
			return nil, errors.New("unknown token: " + tokens[i])
		}
	}

	return stmt, nil
}

func (c *Connection) handleDescribe(ctx context.Context, query string) (rdbms_utils.Rows, error) {
	// Допустим, поддерживаем синтаксис: DESCRIBE TABLE <tableName>
	// Или просто DESCRIBE <tableName>

	// Уберём лишние пробелы, приведём к upper, чтобы было проще искать
	u := strings.ToUpper(strings.TrimSpace(query))

	// Ищем "DESCRIBE TABLE " или "DESCRIBE "
	var tableName string
	if strings.HasPrefix(u, "DESCRIBE TABLE ") {
		tableName = strings.TrimSpace(query[len("DESCRIBE TABLE "):])
	} else {
		// Уберём "DESCRIBE "
		tableName = strings.TrimSpace(query[len("DESCRIBE "):])
	}
	if tableName == "" {
		return nil, errors.New("DESCRIBE: no table name specified")
	}

	// Здесь вы можете:
	// 1) Загрузить JSON c описанием таблицы (tableName) по аналогии с Trino
	// 2) Или сформировать "листы колонок" вручную
	//
	// Для демонстрации сделаем фиктивный список колонок (column_name, data_type)

	columns := []struct {
		ColumnName string
		DataType   string
	}{
		// Допустим, "id" varchar, "field1" varchar, ...
		{"id", "varchar"},
		{"field1", "varchar"},
		{"field2", "varchar"},
		{"field3", "varchar"},
	}

	// Теперь упакуем это в "describeRows", чтобы имитировать результат
	var data []map[string]interface{}
	for _, col := range columns {
		rowMap := map[string]interface{}{
			"column_name": col.ColumnName,
			"data_type":   col.DataType,
		}
		data = append(data, rowMap)
	}

	// Вернём rows, где при Scan(&colName, &typeName) можно прочитать эти данные
	fmt.Println("\n================== handleDescribe ==================\n", data)
	return newDescribeRows(data), nil
}

// convertResults преобразует результаты Redis-запросов в срез map[string]interface{}.
func convertResults(results []interface{}) ([]map[string]interface{}, error) {
	var convertedResults []map[string]interface{}

	for _, result := range results {
		switch res := result.(type) {
		case map[string]string:
			convertedResult := make(map[string]interface{})
			for k, v := range res {
				convertedResult[k] = v
			}
			convertedResults = append(convertedResults, convertedResult)
		case []interface{}:
			for _, item := range res {
				if itemMap, ok := item.(map[string]interface{}); ok {
					convertedResults = append(convertedResults, itemMap)
				}
			}
		case map[string]interface{}:
			// SCAN вернул {"keys": []string, "cursor": <uint64>}
			keysVal, ok := res["keys"]
			if !ok {
				continue
			}
			keySlice, ok := keysVal.([]string)
			if !ok {
				return nil, fmt.Errorf("SCAN 'keys' field must be []string, got %T", keysVal)
			}

			for _, fullKey := range keySlice {
				// Допустим, ключи выглядят как "example_table:123"
				// Нам нужно взять "123" -> rowMap["id"] = "123"
				parts := strings.SplitN(fullKey, ":", 2)
				var idVal string
				if len(parts) == 2 {
					idVal = parts[1]
				} else {
					// fallback
					idVal = fullKey
				}

				rowMap := map[string]interface{}{
					"id":     idVal,
					"field1": nil,
					"field2": nil,
					"field3": nil,
				}

				convertedResults = append(convertedResults, rowMap)
			}
		default:
			return nil, fmt.Errorf("unsupported result type: %T", res)
		}
	}

	return convertedResults, nil
}

// RedisCommand - простая структура для хранения команды Redis
type RedisCommand struct {
	Cmd  string
	Args []interface{}
}

// translateToRedisCommands превращает наш AST (SelectStatement) в
// список (потенциально несколько) команд Redis.
func translateToRedisCommands(stmt *SelectStatement) ([]RedisCommand, error) {
	if !stmt.HasWhere {
		return []RedisCommand{
			{
				Cmd:  "SCAN",
				Args: []interface{}{"0"},
			},
		}, nil
	}

	// Если SELECT * - используем HGETALL <key>
	if len(stmt.Columns) == 1 && stmt.Columns[0] == "*" {
		return []RedisCommand{{
			Cmd:  "HGETALL",
			Args: []interface{}{stmt.Key},
		}}, nil
	}

	// Иначе - считаем, что нужно HMGET <key> col1 col2 ...
	args := []interface{}{stmt.Key}
	for _, col := range stmt.Columns {
		args = append(args, col)
	}

	return []RedisCommand{{
		Cmd:  "HMGET",
		Args: args,
	}}, nil
}

// executeRedisCommands - пример, как выполнить Redis-команды через go-redis.
func executeRedisCommands(ctx context.Context, client *redis.Client, cmds []RedisCommand) ([]interface{}, error) {
	var results []interface{}
	fmt.Println("\nCOMMANDS: ", cmds)
	for _, c := range cmds {
		switch strings.ToUpper(c.Cmd) {
		case "HGETALL":
			if len(c.Args) != 1 {
				return nil, fmt.Errorf("HGETALL expects exactly 1 argument, got %d", len(c.Args))
			}
			key, ok := c.Args[0].(string)
			if !ok {
				return nil, fmt.Errorf("HGETALL key must be string, got %T", c.Args[0])
			}
			res, err := client.HGetAll(ctx, key).Result()
			if err != nil {
				return nil, err
			}
			results = append(results, res)

		case "HMGET":
			if len(c.Args) < 2 {
				return nil, fmt.Errorf("HMGET expects key + at least one field")
			}
			key, ok := c.Args[0].(string)
			if !ok {
				return nil, fmt.Errorf("HMGET key must be string, got %T", c.Args[0])
			}
			fields := make([]string, 0, len(c.Args)-1)
			for _, f := range c.Args[1:] {
				fs, isStr := f.(string)
				if !isStr {
					return nil, fmt.Errorf("HMGET fields must be string, got %T", f)
				}
				fields = append(fields, fs)
			}
			res, err := client.HMGet(ctx, key, fields...).Result()
			if err != nil {
				return nil, err
			}
			results = append(results, res)

		case "SCAN":
			// Сканируем ключи
			if len(c.Args) < 1 {
				return nil, fmt.Errorf("SCAN expects at least 1 argument, got %d", len(c.Args))
			}
			cursorStr, ok := c.Args[0].(string)
			if !ok {
				return nil, fmt.Errorf("SCAN cursor must be string, got %T", c.Args[0])
			}
			cursor64, err := strconv.ParseUint(cursorStr, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid cursor value %q: %w", cursorStr, err)
			}

			keys, _, err := client.Scan(ctx, cursor64, "*", 100).Result()
			if err != nil {
				return nil, err
			}
			sort.Strings(keys)
			// Для каждого найденного ключа делаем HGETALL (или HMGET, если надо только поля).
			// В результате results[] будет набор map[string]string для каждого ключа.
			for _, fullKey := range keys {
				// Optionally parse ID from "example_table:xxx".
				// Then HGETALL:
				hgetAllMap, err := client.HGetAll(ctx, fullKey).Result()
				if err != nil {
					return nil, fmt.Errorf("failed to HGETALL %q: %w", fullKey, err)
				}
				// Храним "сырое" map[string]string, потом convertResults разберётся
				// (или мы сразу собираем map[string]interface{}).
				rowMap := map[string]string{
					"id": fullKey, // техническое поле, чтобы знать откуда взяли
				}
				for k, v := range hgetAllMap {
					rowMap[k] = v
				}
				results = append(results, rowMap)
			}

		default:
			return nil, fmt.Errorf("unsupported command: %s", c.Cmd)
		}
	}
	return results, nil
}
