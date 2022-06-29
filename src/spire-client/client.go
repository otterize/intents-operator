package spire_client

import (
	spireServerUtil "github.com/spiffe/spire/cmd/spire-server/util"
	spireCommonUtil "github.com/spiffe/spire/pkg/common/util"
)

func NewServerClientFromUnixSocket(socketPath string) (spireServerUtil.ServerClient, error) {
	addr, err := spireCommonUtil.GetUnixAddrWithAbsPath(socketPath)
	if err != nil {
		return nil, err
	}

	c, err := spireServerUtil.NewServerClient(addr)
	if err != nil {
		panic(err)
	}

	return c, nil
}
