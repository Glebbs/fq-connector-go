package redis

import (
	"errors"
	"fmt"

	"github.com/ydb-platform/fq-connector-go/app/server/conversion"
	"github.com/ydb-platform/fq-connector-go/app/server/paging"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb"

	rdbms_utils "github.com/ydb-platform/fq-connector-go/app/server/datasource/rdbms/utils"
)

// describeRows хранит набор строк вида [{column_name: "id", data_type: "varchar"}, ...]
type describeRows struct {
	data []map[string]interface{}
	idx  int
}

func newDescribeRows(data []map[string]interface{}) rdbms_utils.Rows {
	return &describeRows{data: data, idx: -1}
}

func (r *describeRows) NextResultSet() bool {
	return false
}

func (r *describeRows) Next() bool {
	r.idx++
	return r.idx < len(r.data)
}

func (r *describeRows) Err() error {
	return nil
}

func (r *describeRows) Scan(dest ...any) error {
	if r.idx < 0 || r.idx >= len(r.data) {
		return errors.New("no row to scan")
	}
	row := r.data[r.idx]

	// Проверяем, что ровно 2 поля (column_name, data_type)
	if len(dest) != 2 {
		return fmt.Errorf("describeRows Scan expects exactly 2 destinations, got %d", len(dest))
	}

	// --- Достаем dest[0] как **string ---
	colNameDoublePtr, ok := dest[0].(**string)
	if !ok {
		return fmt.Errorf("dest[0] must be **string (pointer to pointer to string)")
	}

	// --- Достаем dest[1] как **string ---
	typeNameDoublePtr, ok := dest[1].(**string)
	if !ok {
		return fmt.Errorf("dest[1] must be **string (pointer to pointer to string)")
	}

	// Из row["column_name"] берем значение (string или пустая строка)
	colNameVal, _ := row["column_name"].(string)
	// Аналогично для row["data_type"]
	typeNameVal, _ := row["data_type"].(string)

	// Создаем новые *string
	// и присваиваем им извлеченные значения
	colNamePtr := new(string)
	*colNamePtr = colNameVal

	typeNamePtr := new(string)
	*typeNamePtr = typeNameVal

	// Записываем в исходные переменные (которые объявлены как *string)
	*colNameDoublePtr = colNamePtr
	*typeNameDoublePtr = typeNamePtr

	return nil
}

func (r *describeRows) Close() error {
	return nil
}

// ColumnTypes - не используем (или возвращаем nil)
func (r *describeRows) ColumnTypes() ([]string, error) {
	return nil, nil
}

func (r *describeRows) MakeTransformer(_ []*Ydb.Type, _ conversion.Collection) (paging.RowTransformer[any], error) {
	return nil, fmt.Errorf("not implemented")
}
