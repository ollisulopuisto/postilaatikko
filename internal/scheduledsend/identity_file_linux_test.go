//go:build linux

package scheduledsend

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestFileIdentitySurvivesReopenAndCannotReplace(t *testing.T) {
	s := FileIdentityStore{Directory: privateIdentityTemp(t)}
	i, err := s.Create("studio")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := (FileIdentityStore{Directory: s.Directory}).Load("studio")
	if err != nil || restored.PublicKey() != i.PublicKey() {
		t.Fatal("restart lost identity")
	}
	_, path, _ := s.paths("studio")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, i.private[:]) {
		t.Fatal("plaintext private key")
	}
	if _, err = s.Create("studio"); err == nil {
		t.Fatal("replaced existing identity")
	}
	again, err := s.Load("studio")
	if err != nil || again.PublicKey() != i.PublicKey() {
		t.Fatal("failed create changed identity")
	}
}
func TestFileIdentityCorruptionAndLostKeyNeverRegenerate(t *testing.T) {
	for _, mode := range []string{"corrupt", "missing-key", "public-file", "symlink", "wrong-name"} {
		t.Run(mode, func(t *testing.T) {
			s := FileIdentityStore{Directory: privateIdentityTemp(t)}
			if _, err := s.Create("studio"); err != nil {
				t.Fatal(err)
			}
			dir, path, _ := s.paths("studio")
			switch mode {
			case "corrupt":
				os.WriteFile(path, []byte("corrupt"), 0600)
			case "missing-key":
				os.Remove(filepath.Join(dir, "device.key"))
			case "public-file":
				os.Chmod(path, 0644)
			case "symlink":
				data, _ := os.ReadFile(path)
				other := filepath.Join(t.TempDir(), "private")
				os.WriteFile(other, data, 0600)
				os.Remove(path)
				os.Symlink(other, path)
			case "wrong-name":
				_, other, _ := s.paths("different")
				data, _ := os.ReadFile(path)
				os.WriteFile(other, data, 0600)
				path = other
			}
			name := "studio"
			if mode == "wrong-name" {
				name = "different"
			}
			before, _ := os.ReadFile(path)
			if _, err := s.Load(name); err == nil {
				t.Fatal("accepted unsafe identity")
			}
			if _, err := s.Create(name); err == nil {
				t.Fatal("replaced invalid identity")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("failed load/create changed identity")
			}
			if mode == "missing-key" {
				if _, err := os.Stat(filepath.Join(dir, "device.key")); !os.IsNotExist(err) {
					t.Fatal("regenerated lost encryption key")
				}
			}
		})
	}
}
func TestFileIdentityRejectsPublicOrSymlinkDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "send-later-identities")
	os.Mkdir(dir, 0755)
	if _, err := (FileIdentityStore{Directory: root}).Create("studio"); err == nil {
		t.Fatal("accepted shared directory")
	}
	os.Remove(dir)
	os.Symlink(t.TempDir(), dir)
	if _, err := (FileIdentityStore{Directory: root}).Create("studio"); err == nil {
		t.Fatal("accepted symlink directory")
	}
}

func TestFileIdentityConcurrentInitializersPublishOneKey(t *testing.T) {
	s := FileIdentityStore{Directory: privateIdentityTemp(t)}
	type result struct {
		i   *Identity
		err error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() { i, err := s.Create("studio"); results <- result{i, err} }()
	}
	var winner *Identity
	success := 0
	for range 2 {
		r := <-results
		if r.err == nil {
			success++
			winner = r.i
		}
	}
	if success != 1 || winner == nil {
		t.Fatalf("success count=%d", success)
	}
	restored, err := s.Load("studio")
	if err != nil || restored.PublicKey() != winner.PublicKey() {
		t.Fatal("concurrent setup changed identity")
	}
}

func privateIdentityTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestReloadedFileIdentityDecryptsExistingCommand(t *testing.T) {
	s := FileIdentityStore{Directory: privateIdentityTemp(t)}
	helper, err := s.Create("studio")
	if err != nil {
		t.Fatal(err)
	}
	client, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	command := baselineCommand()
	encoded, err := SealCommand(command, client, helper.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	helper, err = s.Load("studio")
	if err != nil {
		t.Fatal(err)
	}
	restored, sender, err := OpenCommand(encoded, helper)
	if err != nil || sender != client.PublicKey() || restored.JobID != command.JobID {
		t.Fatal("reload cannot decrypt original queue command")
	}
}

func TestFileCreateRejectsExposedEncryptionKeyWithoutRepair(t *testing.T) {
	s := FileIdentityStore{Directory: privateIdentityTemp(t)}
	if _, err := s.Create("first"); err != nil {
		t.Fatal(err)
	}
	dir, second, _ := s.paths("second")
	key := filepath.Join(dir, "device.key")
	if err := os.Chmod(key, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create("second"); err == nil {
		t.Fatal("created identity with exposed encryption key")
	}
	if _, err := os.Stat(second); !os.IsNotExist(err) {
		t.Fatal("published second identity")
	}
	info, err := os.Stat(key)
	if err != nil || info.Mode().Perm() != 0644 {
		t.Fatal("silently repaired exposed key")
	}
}
