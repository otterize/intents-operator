package postgres

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/otterize/intents-operator/src/shared/errors"
	"net"
)

var ErrUndefinedObject = errors.New("undefined object")

func TranslatePostgresConnectionError(err error) (string, bool) {
	if opErr := &(net.OpError{}); errors.As(err, &opErr) || errors.Is(err, context.DeadlineExceeded) {
		return "Can't reach the server", true
	}

	if connErr := &(pgconn.ConnectError{}); errors.As(err, &connErr) {
		return "Can't reach the server", true
	}

	if dnsErr := &(net.DNSError{}); errors.As(err, &dnsErr) {
		return "Can't resolve hostname", true
	}

	if pgErr := &(pgconn.PgError{}); errors.As(err, &pgErr) {
		// See: https://www.postgresql.org/docs/current/errcodes-appendix.html
		if pgErr.Code == "28P01" || pgErr.Code == "28000" {
			return "Invalid credentials", true
		}
		if pgErr.Code == "3D000" {
			return fmt.Sprintf("Database doesn't exist: %s", pgErr.Message), true
		}
	}

	return "", false
}

func IsInvalidAuthorizationError(err error) bool {
	if pgErr := &(pgconn.PgError{}); errors.As(err, &pgErr) {
		if pgErr.Code == "28000" {
			return true
		}
	}
	return false
}

func TranslatePostgresCommandsError(err error) error {
	if pgErr := &(pgconn.PgError{}); errors.As(err, &pgErr) {
		// See: https://www.postgresql.org/docs/current/errcodes-appendix.html
		if pgErr.Code == "42P01" || pgErr.Code == "3F000" {
			return errors.Wrap(fmt.Errorf("bad schema/table name: %s", pgErr.Message))
		}
		if pgErr.Code == "42704" {
			return errors.Wrap(ErrUndefinedObject)
		}
	}
	return errors.Wrap(err)
}
