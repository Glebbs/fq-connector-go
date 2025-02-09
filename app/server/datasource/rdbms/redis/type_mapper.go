// app/server/datasource/rdbms/redis/type_mapper.go

package redis

import (
	"errors"
	"fmt"

	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb"

	"github.com/apache/arrow/go/v13/arrow/array"
	api_service_protos "github.com/ydb-platform/fq-connector-go/api/service/protos"
	"github.com/ydb-platform/fq-connector-go/app/server/conversion"
	"github.com/ydb-platform/fq-connector-go/app/server/datasource"
	"github.com/ydb-platform/fq-connector-go/app/server/paging"
	"github.com/ydb-platform/fq-connector-go/common"
)

var _ datasource.TypeMapper = typeMapper{}

type typeMapper struct{}

func (typeMapper) SQLTypeToYDBColumn(columnName, typeName string, rules *api_service_protos.TTypeMappingSettings) (*Ydb.Column, error) {
	var ydbType *Ydb.Type

	switch typeName {
	case "string", "varchar":
		ydbType = common.MakePrimitiveType(Ydb.Type_UTF8)
	case "int":
		ydbType = common.MakePrimitiveType(Ydb.Type_INT64)
	case "float":
		ydbType = common.MakePrimitiveType(Ydb.Type_DOUBLE)
	case "bool":
		ydbType = common.MakePrimitiveType(Ydb.Type_BOOL)
	// case "list":
	// 	ydbType = &Ydb.Type{
	// 		Type: &Ydb.Type_ListType{
	// 			ListType: &Ydb.ListType{
	// 				Item: &Ydb.Type{
	// 					Type: &Ydb.Type_Primitive_{Primitive: Ydb.Type_STRING}, // Предполагаем, что элементы списка - строки
	// 				},
	// 			},
	// 		},
	// 	}
	// 	ydbType = common.MakePrimitiveType(Ydb.Type_INT64)
	// case "hash":
	// 	ydbType = &Ydb.Type{
	// 		Type: &Ydb.Type_MapType{
	// 			MapType: &Ydb.MapType{
	// 				Key: &Ydb.Type{
	// 					Type: &Ydb.Type_Primitive_{Primitive: Ydb.Type_STRING},
	// 				},
	// 				Value: &Ydb.Type{
	// 					Type: &Ydb.Type_Primitive_{Primitive: Ydb.Type_STRING}, // Предполагаем, что значения - строки
	// 				},
	// 			},
	// 		},
	// 	}
	// 	ydbType = common.MakePrimitiveType(Ydb.Type_INT64)
	default:
		return nil, fmt.Errorf("unsupported Redis type: %s", typeName)
	}

	return &Ydb.Column{
		Name: columnName,
		Type: ydbType,
	}, nil
}

func transformerFromRedisTypes(
	types []string,
	ydbTypes []*Ydb.Type,
	cc conversion.Collection,
) (paging.RowTransformer[any], error) {

	// Проверим, что длины срезов совпадают
	if len(types) != len(ydbTypes) {
		return nil, fmt.Errorf(
			"transformerFromRedisTypes: mismatch in number of Redis vs YDB types: %d vs %d",
			len(types), len(ydbTypes),
		)
	}

	// Срез «приёмников»
	acceptors := make([]any, 0, len(types))
	// Срез «аппендеров»
	appenders := make([]func(acceptor any, builder array.Builder) error, 0, len(types))

	// Идём по каждому Redis-типу
	for _, typeName := range types {
		switch typeName {
		case "string", "varchar":
			// Создаём acceptor: **string
			acceptors = append(acceptors, new(*string))

			// Создаём appender:
			// Укладывает string → string в *array.StringBuilder
			// cc.String() даёт conversion.ValuePtrConverter[string,string]
			appenders = append(appenders, makeAppender[string, string, *array.StringBuilder](cc.String()))

		case "int":
			// Acceptors: **int64
			acceptors = append(acceptors, new(*int64))

			// Укладывать int64 → int64 в Arrow
			// cc.Int64() даёт conversion.ValuePtrConverter[int64,int64]
			appenders = append(
				appenders,
				makeAppender[int64, int64, *array.Int64Builder](cc.Int64()),
			)

		case "float":
			// Acceptors: **float64
			acceptors = append(acceptors, new(*float64))

			// Укладывать float64 → float64
			appenders = append(
				appenders,
				makeAppender[float64, float64, *array.Float64Builder](cc.Float64()),
			)

		case "bool":
			// Acceptors: **bool
			acceptors = append(acceptors, new(*bool))

			// Bool в Arrow часто хранится как uint8 (0 или 1) либо bit-поле.
			// В FQ-коннекторе обычно делается так:
			appenders = append(
				appenders,
				makeAppender[bool, uint8, *array.Uint8Builder](cc.Bool()),
			)

		default:
			return nil, fmt.Errorf("unsupported Redis type for transformer: %s", typeName)
		}
	}

	// Собираем RowTransformer
	// Параметры:
	//  1) acceptors — куда Scan() запишет данные
	//  2) appenders — как переводить acceptors → Arrow builder
	//  3) (опциональный) finalizer, если нужно
	return paging.NewRowTransformer[any](acceptors, appenders, nil), nil
}

func makeAppender[
	IN common.ValueType,
	OUT common.ValueType,
	AB common.ArrowBuilder[OUT],
](conv conversion.ValuePtrConverter[IN, OUT]) func(acceptor any, builder array.Builder) error {
	return func(acceptor any, builder array.Builder) error {
		return appendValueToArrowBuilder[IN, OUT, AB](acceptor, builder, conv)
	}
}

func appendValueToArrowBuilder[IN common.ValueType, OUT common.ValueType, AB common.ArrowBuilder[OUT]](
	acceptor any,
	builder array.Builder,
	conv conversion.ValuePtrConverter[IN, OUT],
) error {
	cast := acceptor.(**IN)

	if *cast == nil {
		builder.AppendNull()
		return nil
	}

	value := *cast

	out, err := conv.Convert(value)
	if err != nil {
		if errors.Is(err, common.ErrValueOutOfTypeBounds) {
			// TODO: написать предупреждение в логгер
			builder.AppendNull()
			return nil
		}
		return fmt.Errorf("convert value %v: %w", value, err)
	}

	//nolint:forcetypeassert
	builder.(AB).Append(out)

	// Значение было скопировано из ClickHouse, не уверен, необходимо ли это
	*cast = nil

	return nil
}

func NewTypeMapper() datasource.TypeMapper { return typeMapper{} }
