package identity

import "testing"

func TestPasswordUsesBcrypt(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if matched, upgrade := CheckPassword("correct horse battery staple", hash); !matched || upgrade {
		t.Fatalf("bcrypt password did not verify: matched=%v upgrade=%v", matched, upgrade)
	}
	if matched, _ := CheckPassword("wrong", hash); matched {
		t.Fatal("wrong password matched")
	}
}

func TestLegacySHA256RequestsUpgrade(t *testing.T) {
	legacy := HashToken("legacy-password")
	if matched, upgrade := CheckPassword("legacy-password", legacy); !matched || !upgrade {
		t.Fatalf("legacy password behavior mismatch: matched=%v upgrade=%v", matched, upgrade)
	}
}
