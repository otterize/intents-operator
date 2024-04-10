package graphqlclient

import _ "github.com/suessflorian/gqlfetch"

//go:generate sh -c "go run github.com/suessflorian/gqlfetch/gqlfetch --endpoint http://localhost:8080/api/graphql/v1beta > schema.graphql"
//go:generate go run github.com/Khan/genqlient ./genqlient.yaml
