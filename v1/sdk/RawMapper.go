package sdk

import (
	"github.com/IUAD1IY7/opa/v1/rego"
)

type RawMapper struct {
}

func (*RawMapper) MapResults(pq *rego.PartialQueries) (any, error) {

	return pq, nil
}

func (*RawMapper) ResultToJSON(results any) (any, error) {
	return results, nil
}
