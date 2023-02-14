package otterizegraphql

import _ "github.com/suessflorian/gqlfetch"

//go:generate sh -c "go run github.com/suessflorian/gqlfetch/gqlfetch --endpoint https://app.staging.otterize.com/api/graphql/v1beta > schema.graphql"
//go:generate go run github.com/Khan/genqlient ./genqlient.yaml
