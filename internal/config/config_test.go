package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMySQLDefaultsFileReadsOnlyClientSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mysql.cnf")
	content := "[other]\nuser=wrong\n[client]\nuser=ledger\npassword=private\nsocket=/tmp/mysql.sock\ndatabase=shoreos\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := loadMySQLDefaultsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["user"] != "ledger" || values["socket"] != "/tmp/mysql.sock" || values["database"] != "shoreos" {
		t.Fatalf("unexpected client values: %#v", values)
	}
}
