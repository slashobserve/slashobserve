package db_test

import (
	"io/fs"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const firstLockCheckedMigration = 76

var (
	alterClause      = regexp.MustCompile(`,\s*\n\s*`)
	columnConstraint = regexp.MustCompile(`(?is)^\s*(ALTER TABLE \S+\s+)?ADD COLUMN\b.*\b(REFERENCES|CHECK)\b`)
	addedConstraint  = regexp.MustCompile(`(?is)^\s*(ALTER TABLE \S+\s+)?ADD CONSTRAINT (\w+)\s+(CHECK|FOREIGN KEY)\b`)
)

func TestNewMigrationsCheckExistingRowsOutsideTheTableLock(t *testing.T) {
	migrations := os.DirFS("migrations")
	files, err := fs.Glob(migrations, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		version, err := strconv.Atoi(strings.SplitN(path.Base(file), "_", 2)[0])
		if err != nil || version < firstLockCheckedMigration {
			continue
		}
		body, err := fs.ReadFile(migrations, file)
		if err != nil {
			t.Fatal(err)
		}
		_, up, _ := strings.Cut(string(body), "-- +goose Up\n")
		up, _, _ = strings.Cut(up, "-- +goose Down\n")
		statements := migrationStatements(up)
		validated := strings.Join(statements, "\n")
		for _, statement := range statements {
			if !strings.HasPrefix(strings.TrimSpace(statement), "ALTER TABLE") {
				continue
			}
			for _, clause := range alterClause.Split(statement, -1) {
				if columnConstraint.MatchString(clause) {
					t.Errorf("%s: a column added with a constraint is checked under the table lock; add the column, then the constraint NOT VALID and VALIDATE it:\n%s", file, clause)
				}
				m := addedConstraint.FindStringSubmatch(clause)
				if m == nil {
					continue
				}
				if !strings.Contains(clause, "NOT VALID") {
					t.Errorf("%s: %s is checked under the table lock; add it NOT VALID and VALIDATE it", file, m[2])
				}
				if !strings.Contains(validated, "VALIDATE CONSTRAINT "+m[2]) {
					t.Errorf("%s: %s is never validated", file, m[2])
				}
				if !strings.Contains(string(body), "-- +goose NO TRANSACTION") {
					t.Errorf("%s: %s is validated inside the transaction that holds the table lock; mark the migration NO TRANSACTION", file, m[2])
				}
			}
		}
	}
}
