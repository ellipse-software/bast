package keys

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (m Manager) GenerateCommand(name, algorithm string) (*exec.Cmd, string, error) {
	if err := validateName(name); err != nil {
		return nil, "", err
	}
	if algorithm == "" {
		algorithm = "ed25519"
	}
	if algorithm != "ed25519" && algorithm != "rsa" {
		return nil, "", errors.New("algorithm must be ed25519 or rsa")
	}
	if err := os.MkdirAll(m.Paths.ManagedKeys, 0700); err != nil {
		return nil, "", err
	}
	path := filepath.Join(m.Paths.ManagedKeys, name)
	if regular(path) || regular(path+".pub") {
		return nil, "", fmt.Errorf("key %q already exists", name)
	}
	args := []string{"-t", algorithm, "-f", path, "-C", name}
	if algorithm == "rsa" {
		args = append(args, "-b", "4096")
	}
	cmd := exec.Command(m.SSHKeygen, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd, path, nil
}

func (m Manager) PassphraseCommand(key Key) (*exec.Cmd, error) {
	if key.PrivatePath == "" {
		return nil, errors.New("this key has no private file")
	}
	cmd := exec.Command(m.SSHKeygen, "-p", "-f", key.PrivatePath)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd, nil
}

func (m Manager) AgentCommand(key Key, load bool) (*exec.Cmd, error) {
	if key.PrivatePath == "" {
		return nil, errors.New("this key has no private file")
	}
	args := []string{key.PrivatePath}
	if !load {
		args = append([]string{"-d"}, args...)
	}
	cmd := exec.Command(m.SSHAdd, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd, nil
}

func (m Manager) Import(privateSource, publicSource, name, comment string) error {
	if err := validateName(name); err != nil {
		return err
	}
	comment = strings.TrimSpace(comment)
	if strings.ContainsAny(comment, "\r\n") {
		return errors.New("public-key comment cannot contain a newline")
	}
	if err := os.MkdirAll(m.Paths.ManagedKeys, 0700); err != nil {
		return err
	}
	destination := filepath.Join(m.Paths.ManagedKeys, name)
	if regular(destination) || regular(destination+".pub") {
		return fmt.Errorf("key %q already exists", name)
	}

	privatePath, privatePasted, privateCleanup, err := m.importSource(privateSource, true)
	if err != nil {
		return err
	}
	defer privateCleanup()
	privateFingerprint, err := m.fileFingerprint(privatePath)
	if err != nil {
		return fmt.Errorf("private key is not readable by OpenSSH: %w", err)
	}

	publicPath := ""
	publicCleanup := func() {}
	if strings.TrimSpace(publicSource) != "" {
		publicPath, _, publicCleanup, err = m.importSource(publicSource, false)
	} else if !privatePasted && regular(privatePath+".pub") {
		publicPath = privatePath + ".pub"
	} else {
		public, deriveErr := exec.Command(m.SSHKeygen, "-y", "-f", privatePath).CombinedOutput()
		if deriveErr != nil {
			return errors.New("could not derive the public key; paste its public key or place a matching .pub file beside the private key")
		}
		publicPath, _, publicCleanup, err = m.importSource(string(public), false)
	}
	if err != nil {
		return err
	}
	defer publicCleanup()
	publicFingerprint, err := m.fileFingerprint(publicPath)
	if err != nil {
		return fmt.Errorf("public key is not readable by OpenSSH: %w", err)
	}
	if privateFingerprint != publicFingerprint {
		return errors.New("the public key does not match the private key")
	}
	if comment != "" {
		public, readErr := os.ReadFile(publicPath)
		if readErr != nil {
			return readErr
		}
		fields := strings.Fields(string(public))
		if len(fields) < 2 {
			return errors.New("public key has no key material")
		}
		commentedPath, _, commentedCleanup, sourceErr := m.importSource(fields[0]+" "+fields[1]+" "+comment, false)
		if sourceErr != nil {
			return sourceErr
		}
		defer commentedCleanup()
		publicPath = commentedPath
	}

	if err := copyFile(privatePath, destination, 0600); err != nil {
		return err
	}
	if err := copyFile(publicPath, destination+".pub", 0644); err != nil {
		_ = os.Remove(destination)
		return err
	}
	return nil
}

func (m Manager) importSource(source string, private bool) (string, bool, func(), error) {
	content := strings.TrimSpace(source)
	pasted := strings.Contains(content, "PRIVATE KEY-----")
	if !private {
		pasted = strings.HasPrefix(content, "ssh-") || strings.HasPrefix(content, "ecdsa-") || strings.HasPrefix(content, "sk-")
	}
	if !pasted {
		path := expandHome(content, m.Paths.Home)
		if !regular(path) {
			kind := "public"
			if private {
				kind = "private"
			}
			return "", false, func() {}, fmt.Errorf("%s key %q does not exist", kind, path)
		}
		return path, false, func() {}, nil
	}
	if private && !strings.HasPrefix(content, "-----BEGIN ") {
		return "", false, func() {}, errors.New("pasted content is not a private key")
	}
	tmp, err := os.CreateTemp(m.Paths.ManagedKeys, ".import-*")
	if err != nil {
		return "", false, func() {}, err
	}
	path := tmp.Name()
	cleanup := func() { _ = os.Remove(path) }
	mode := os.FileMode(0644)
	if private {
		mode = 0600
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		cleanup()
		return "", false, func() {}, err
	}
	if _, err := tmp.WriteString(content + "\n"); err != nil {
		tmp.Close()
		cleanup()
		return "", false, func() {}, err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", false, func() {}, err
	}
	return path, true, cleanup, nil
}

func (m Manager) fileFingerprint(path string) (string, error) {
	out, err := exec.Command(m.SSHKeygen, "-lf", path).CombinedOutput()
	if err != nil {
		return "", errors.New(strings.TrimSpace(string(out)))
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return "", errors.New("ssh-keygen returned no fingerprint")
	}
	return fields[1], nil
}

func (m Manager) Export(key Key, directory string) error {
	if key.PrivatePath == "" && key.PublicPath == "" {
		return errors.New("this agent key has no exportable key file")
	}
	directory = expandHome(directory, m.Paths.Home)
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		return fmt.Errorf("export destination must be an existing directory")
	}
	name := key.Name
	if key.PrivatePath != "" && regular(filepath.Join(directory, name)) {
		return fmt.Errorf("refusing to overwrite %s", filepath.Join(directory, name))
	}
	if key.PublicPath != "" && regular(filepath.Join(directory, name+".pub")) {
		return fmt.Errorf("refusing to overwrite %s", filepath.Join(directory, name+".pub"))
	}
	if key.PrivatePath != "" {
		destination := filepath.Join(directory, name)
		if err := copyFile(key.PrivatePath, destination, 0600); err != nil {
			return err
		}
	}
	if key.PublicPath != "" {
		destination := filepath.Join(directory, name+".pub")
		if err := copyFile(key.PublicPath, destination, 0644); err != nil {
			return err
		}
	}
	return nil
}

func (m Manager) Delete(key Key, confirmation string) error {
	if key.PrivatePath == "" && key.PublicPath == "" {
		return errors.New("this agent key has no key file to delete")
	}
	if confirmation != key.Name {
		return errors.New("confirmation did not match the exact key name")
	}
	if len(key.References) > 0 {
		return fmt.Errorf("key is still referenced by: %s", strings.Join(key.References, ", "))
	}
	if key.InAgent && key.PrivatePath != "" {
		_ = exec.Command(m.SSHAdd, "-d", key.PrivatePath).Run()
	}
	if key.PrivatePath != "" {
		if err := os.Remove(key.PrivatePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if key.PublicPath != "" {
		if err := os.Remove(key.PublicPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func PublicText(key Key) (string, error) {
	if key.PublicPath == "" {
		return "", errors.New("no public key file is available")
	}
	b, err := os.ReadFile(key.PublicPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (m Manager) SetComment(key Key, comment string) error {
	comment = strings.TrimSpace(comment)
	if strings.ContainsAny(comment, "\r\n") {
		return errors.New("public-key comment cannot contain a newline")
	}
	publicPath := key.PublicPath
	var public []byte
	var err error
	if publicPath != "" {
		if !within(publicPath, m.Paths.ManagedKeys) {
			return errors.New("external keys cannot be edited by Bast")
		}
		public, err = os.ReadFile(publicPath)
		if err != nil {
			return err
		}
	} else {
		if key.PrivatePath == "" || !within(key.PrivatePath, m.Paths.ManagedKeys) {
			return errors.New("this key has no Bast-managed private key")
		}
		public, err = exec.Command(m.SSHKeygen, "-y", "-f", key.PrivatePath).CombinedOutput()
		if err != nil {
			return errors.New("could not derive the public key; it may require a passphrase")
		}
		publicPath = key.PrivatePath + ".pub"
	}
	fields := strings.Fields(string(public))
	if len(fields) < 2 {
		return errors.New("public key has no key material")
	}
	line := fields[0] + " " + fields[1]
	if comment != "" {
		line += " " + comment
	}
	line += "\n"

	tmp, err := os.CreateTemp(filepath.Dir(publicPath), ".comment-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(line); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, publicPath)
}
