package redis

import (
	"fmt"

	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb"

	"github.com/ydb-platform/fq-connector-go/app/server/conversion"
	"github.com/ydb-platform/fq-connector-go/app/server/paging"
	// Предположим, что rdbms_utils.Rows определён в вашем проекте как общий интерфейс
	// "github.com/ydb-platform/fq-connector-go/app/server/datasource/rdbms/utils"
)

// rows хранит результаты одного или нескольких Redis-запросов.
// В примере предполагается, что мы сохранили "табличные" данные (например, HGETALL/SCAN)
// в срез `data`. Каждый элемент — это мапа «имя_поля -> значение».
type rows struct {
	result []map[string]interface{} // Ваши «строки»
	idx    int                      // Текущий индекс, если мы идём по строкам
	err    error                    // Ошибка, если возникла
}

// NextResultSet — аналогично SQL, но Redis в большинстве случаев не возвращает несколько result set.
// Можно вернуть false, если у вас одна выборка. Или расширить логику, если у вас есть команды,
// возвращающие несколько наборов данных.
func (r *rows) NextResultSet() bool {
	// Если вы хотите поддерживать несколько наборов, здесь можно переключаться между ними
	return false
}

// Next переходит к следующей «строке» в r.data.
func (r *rows) Next() bool {
	if r.idx >= len(r.result) {
		return false
	}
	r.idx++
	return true
}

// Err возвращает ошибку, которая могла произойти при получении / разборе данных.
func (r *rows) Err() error {
	return r.err
}

// ColumnTypes возвращает список «типов столбцов», если вам нужно что-то аналогичное SQL.
// В Redis зачастую нет информации о типах полей — всё хранится как строки или сериализованные данные.
// Поэтому можно вернуть nil или собрать типы по содержимому `data`.
func (r *rows) ColumnTypes() ([]string, error) {
	if r.idx == 0 || r.idx > len(r.result) {
		// Ещё не вызван Next() или мы уже вышли за пределы
		return nil, nil
	}
	row := r.result[r.idx-1]
	typeNames := make([]string, 0, len(row))
	for fieldName, fieldValue := range row {
		// Условно возьмём Go-тип и переведём его в строку
		// (например, "string", "int", "float64" и т.д.)
		typeName := fmt.Sprintf("%T", fieldValue)
		typeNames = append(typeNames, fmt.Sprintf("%s:%s", fieldName, typeName))
	}
	return typeNames, nil
}

// Scan — аналог SQL Scan: заполняет dest значениями из текущей «строки».
// Для упрощения дадим пользоваться именами полей (похоже на map-структуру).
// Обычно в SQL Scan идёт по позициям (dest[0] -> первая колонка).
// Если хотите точь-в-точь как в SQL, храните порядок колонок где-то отдельно.
func (r *rows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx > len(r.result) {
		return fmt.Errorf("no row to scan, call Next() first or we're out of rows")
	}
	row := r.result[r.idx-1]

	// Проверяем, что dest имеет хотя бы столько же элементов,
	// сколько полей в строке (или можно быть более гибкими).
	if len(dest) != len(row) {
		return fmt.Errorf(
			"Scan: mismatch columns count: expected %d, got %d",
			len(row), len(dest),
		)
	}
	fmt.Println("\nScan: result: ", r.result)
	fmt.Println("\nScan: row: ", row)
	i := 0
	for fieldName, fieldValue := range row {
		switch ptr := dest[i].(type) {
		// 1) Случай, когда вызывающая сторона сделала: var colName *string; Scan(&colName)
		//    Тогда в switch мы увидим ptr как (**string).
		//    Надо обработать это как «указатель на указатель на string».
		case **string:
			// Создаём новый *string
			stringPtr := new(string)

			// Преобразуем fieldValue, например, в строку
			// (если в Redis действительно хранился string)
			strVal, ok := fieldValue.(string)
			if !ok {
				return fmt.Errorf("expected string for field %q, got %T", fieldName, fieldValue)
			}
			*stringPtr = strVal

			// Записываем обратно
			*ptr = stringPtr

		// 2) Случай, когда вызывающая сторона сделала: var colName string; Scan(&colName)
		//    Тогда в switch ptr имеет тип (*string).
		case *string:
			strVal, ok := fieldValue.(string)
			if !ok {
				return fmt.Errorf("expected string for field %q, got %T", fieldName, fieldValue)
			}
			*ptr = strVal

		// 3) Случай, когда вызывающая сторона сделала: var colInt int; Scan(&colInt)
		//    ptr будет (*int)
		case *int:
			intVal, ok := fieldValue.(int)
			if !ok {
				return fmt.Errorf("expected int for field %q, got %T", fieldName, fieldValue)
			}
			*ptr = intVal

		// 4) Аналогично для других типов (float64, bool, и т.д.).
		//    Или для «pointer to pointer» ситуаций: **int, **bool, и т.д., если вдруг есть.
		default:
			return fmt.Errorf("unsupported dest type %T for field %s", ptr, fieldName)
		}
		i++
	}

	return nil
}

// Close — закрывает «курсор». В Redis нет открытых курсоров,
// но если вы сканировали большие объёмы (через SCAN), можно тут освободить ресурсы.
func (r *rows) Close() error {
	// no-op или что-то, если нужно
	return nil
}

// MakeTransformer — аналог того, что мы видим в ms_sql_server.
// Обычно он создаёт некую функцию-трансформер, которая будет
// конвертировать одну «строку» (map[string]interface{}) в структуру YDB (Paging).
func (r *rows) MakeTransformer(ydbTypes []*Ydb.Type, cc conversion.Collection) (paging.RowTransformer[any], error) {
	// Если вообще нет данных, не из чего выводить схему
	if len(r.result) == 0 {
		return nil, fmt.Errorf("no data to infer columns")
	}

	// Берём первую строку как «образец»
	sampleRow := r.result[0]

	// В этот же момент выведем «тип» (string, int, float, bool…) для каждого поля
	typeNames := make([]string, 0, len(sampleRow))

	for _, fieldValue := range sampleRow {
		// typeNames = append(typeNames, fieldName)

		// Простейший вариант свитча по Go-типу:
		switch fieldValue.(type) {
		case string:
			typeNames = append(typeNames, "string")
		case int, int32, int64:
			typeNames = append(typeNames, "int")
		case float32, float64:
			typeNames = append(typeNames, "float")
		case bool:
			typeNames = append(typeNames, "bool")
		default:
			// По умолчанию можно расценивать как string,
			// или возвращать ошибку, если хотите быть строже.
			typeNames = append(typeNames, "string")
		}
	}

	// Теперь у нас есть columns[] и typeNames[], соответствующие полям sampleRow.
	// Вызываем нашу функцию, которая построит RowTransformer.
	fmt.Println("\n\nTYPES: ", typeNames, ydbTypes, sampleRow)
	transformer, err := transformerFromRedisTypes(typeNames, ydbTypes, cc)
	if err != nil {
		return nil, fmt.Errorf("transformer from redis types: %w", err)
	}

	return transformer, nil
}
