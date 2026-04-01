package crypto

import "testing"

func TestEncryptDecrypt(t *testing.T) {
	enc, err := NewEncryptor("test-key-for-encryption")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	tests := []struct {
		name      string
		plaintext string
	}{
		{"empty string", ""},
		{"short string", "hello"},
		{"longer string", "this is a longer secret value with special chars: !@#$%^&*()"},
		{"unicode", "příliš žluťoučký kůň úpěl ďábelské ódy"},
		{"json", `{"key": "value", "number": 42}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := enc.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}

			// Encrypted should be different from plaintext
			if encrypted == tt.plaintext && tt.plaintext != "" {
				t.Error("encrypted value equals plaintext")
			}

			decrypted, err := enc.Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}

			if decrypted != tt.plaintext {
				t.Errorf("got %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	enc, err := NewEncryptor("test-key")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	a, _ := enc.Encrypt("same-value")
	b, _ := enc.Encrypt("same-value")

	if a == b {
		t.Error("same plaintext produced identical ciphertexts (nonce should make them different)")
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	enc1, _ := NewEncryptor("key-one")
	enc2, _ := NewEncryptor("key-two")

	encrypted, _ := enc1.Encrypt("secret")
	_, err := enc2.Decrypt(encrypted)
	if err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

func TestMaskValue(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "****"},
		{"ab", "****"},
		{"abcd", "****"},
		{"abcde", "****bcde"},
		{"my-secret-key-value", "****alue"},
	}

	for _, tt := range tests {
		got := MaskValue(tt.input)
		if got != tt.want {
			t.Errorf("MaskValue(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNewEncryptorEmptyKey(t *testing.T) {
	_, err := NewEncryptor("")
	if err == nil {
		t.Error("expected error for empty key")
	}
}
