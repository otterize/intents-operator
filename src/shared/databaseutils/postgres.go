package databaseutils

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/otterize/intents-operator/src/shared/errors"
	"net"
	"strings"
)

const (
	UsernameMaxLenPreHash = 50
	PGSelectUserQuery     = "SELECT FROM pg_catalog.pg_user where usename = $1"
)

func BuildHashedUsername(workloadName, namespace, clusterUID string) string {
	username := fmt.Sprintf("%s-%s-%s", workloadName, namespace, clusterUID)
	hash := md5.Sum([]byte(username))

	if len(username) > UsernameMaxLenPreHash {
		username = username[:UsernameMaxLenPreHash]
	}
	username = strings.TrimSuffix(username, "-") // Just in case we trimmed at a hyphen separator

	hashSuffix := hex.EncodeToString(hash[:])[:6]
	return fmt.Sprintf("%s-%s", username, hashSuffix)
}

// KubernetesToPostgresName translates a name with Kubernetes conventions to Postgres conventions
func KubernetesToPostgresName(kubernetesName string) string {
	// '.' are replaced with dunders '__'
	// '-' are replaced with single underscores '_'
	return strings.ReplaceAll(strings.ReplaceAll(kubernetesName, ".", "__"), "-", "_")
}

func PostgresToKubernetesName(pgName string) string {
	return strings.ReplaceAll(strings.ReplaceAll(pgName, "__", "."), "_", "-")
}

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
	}
	return errors.Wrap(err)
}

func ValidateUserExists(ctx context.Context, user string, conn *pgx.Conn) (bool, error) {
	row, err := conn.Query(ctx, PGSelectUserQuery, user)
	if err != nil {
		return false, errors.Wrap(err)
	}
	defer row.Close() // "row" either holds 1 or 0 rows, and we must call Close() before reusing the connection again

	return row.Next(), nil
}
