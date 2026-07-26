package remarkable

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// xochitlDir is where reMarkable's document-store UI (xochitl) looks for
// documents. Dropping a file anywhere else (e.g. plain SFTP to $HOME) makes
// it invisible in "My Files".
const xochitlDir = "/home/root/.local/share/remarkable/xochitl"

type Config struct {
	Host     string
	Port     string
	User     string
	Password string
}

type UploadResult struct {
	UUID        string
	VisibleName string
	Bytes       int64
}

// SendEpub uploads epubPath to the tablet, registers it in xochitl's
// document store under title (or the filename if title is empty), and
// restarts xochitl so it picks up, paginates, and shows the new document in
// My Files.
func SendEpub(cfg Config, epubPath, title string) (*UploadResult, error) {
	if err := validateEpub(epubPath); err != nil {
		return nil, err
	}

	info, err := os.Stat(epubPath)
	if err != nil {
		return nil, fmt.Errorf("stat epub: %w", err)
	}

	visibleName := title
	if visibleName == "" {
		visibleName = VisibleNameFromFilename(epubPath)
	}

	uuid, err := NewUUID()
	if err != nil {
		return nil, fmt.Errorf("generate uuid: %w", err)
	}

	sshConn, err := dial(cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial: %w", err)
	}
	defer sshConn.Close()

	sftpClient, err := sftp.NewClient(sshConn)
	if err != nil {
		return nil, fmt.Errorf("open sftp session: %w", err)
	}
	defer sftpClient.Close()

	if err := uploadFile(sftpClient, epubPath, path.Join(xochitlDir, uuid+".epub")); err != nil {
		return nil, fmt.Errorf("upload epub: %w", err)
	}

	metaJSON, err := BuildMetadata(visibleName, time.Now()).JSON()
	if err != nil {
		return nil, fmt.Errorf("build metadata: %w", err)
	}
	if err := writeRemoteFile(sftpClient, path.Join(xochitlDir, uuid+".metadata"), metaJSON); err != nil {
		return nil, fmt.Errorf("write metadata: %w", err)
	}

	contentJSON, err := BuildContent(info.Size()).JSON()
	if err != nil {
		return nil, fmt.Errorf("build content: %w", err)
	}
	if err := writeRemoteFile(sftpClient, path.Join(xochitlDir, uuid+".content"), contentJSON); err != nil {
		return nil, fmt.Errorf("write content: %w", err)
	}

	if err := restartXochitl(sshConn); err != nil {
		return nil, fmt.Errorf("restart xochitl: %w", err)
	}

	return &UploadResult{UUID: uuid, VisibleName: visibleName, Bytes: info.Size()}, nil
}

func validateEpub(p string) error {
	if filepath.Ext(p) != ".epub" {
		return fmt.Errorf("%s is not an .epub file", p)
	}
	if _, err := os.Stat(p); err != nil {
		return fmt.Errorf("cannot access %s: %w", p, err)
	}
	return nil
}

func dial(cfg Config) (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            []ssh.AuthMethod{ssh.Password(cfg.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	return ssh.Dial("tcp", fmt.Sprintf("%s:%s", cfg.Host, cfg.Port), config)
}

func uploadFile(client *sftp.Client, localPath, remotePath string) error {
	src, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := client.Create(remotePath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func writeRemoteFile(client *sftp.Client, remotePath string, data []byte) error {
	dst, err := client.Create(remotePath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = dst.Write(data)
	return err
}

func restartXochitl(conn *ssh.Client) error {
	session, err := conn.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	return session.Run("systemctl restart xochitl")
}
