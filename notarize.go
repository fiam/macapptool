package main

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/google/subcommands"
	"github.com/manifoldco/promptui"

	"howett.net/plist"
)

var (
	statusRe     = regexp.MustCompile("Status: ([\\w ]+)")
	uuidRe       = regexp.MustCompile("RequestUUID = ([0-9a-z\\-]+)")
	logFileURLRe = regexp.MustCompile("LogFileURL: (.*)")
	// returned when resubmitting
	uuidAltRe           = regexp.MustCompile("The upload ID is ([0-9a-z\\-]+)")
	errUnexpectedFormat = errors.New("unexpect output format from altool")
)

type notarizationRequest struct {
	AppPath  string
	Username string
	Password string
	UUID     string
}

func commandDebugString(args ...string) string {
	var values []string
	expectPassword := false
	for _, v := range args {
		if expectPassword {
			values = append(values, strings.Repeat("Xx", 8)+"X")
			expectPassword = false
			continue
		}
		values = append(values, v)
	}
	return strings.Join(values, " ")
}

func writeCommandOutputOnDir(dir string, w io.Writer, args ...string) error {
	cmdString := commandDebugString(args...)
	if dir != "" {
		fmt.Printf("(%s) @%s\n", dir, cmdString)
	} else {
		fmt.Printf("@%s\n", cmdString)
	}
	cmd := exec.Command(args[0], args[1:]...)
	if dir != "" {
		cmd.Dir = dir
	}
	var (
		stdout io.Writer = os.Stdout
		stderr io.Writer = os.Stderr
	)
	if w != nil {
		stdout = io.MultiWriter(stdout, w)
		stderr = io.MultiWriter(stderr, w)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func runCommandOnDir(dir string, args ...string) error {
	return writeCommandOutputOnDir(dir, nil, args...)
}

func writeCommandOutput(w io.Writer, args ...string) error {
	return writeCommandOutputOnDir("", w, args...)
}

func runCommand(args ...string) error {
	return runCommandOnDir("", args...)
}

func staple(zipFile string) error {
	// xcrun stapler staple
	dir, err := ioutil.TempDir("", "notarizer")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	appPath, err := unzipApp(zipFile, dir)
	if err != nil {
		return err
	}
	if err := runCommand("xcrun", "stapler", "staple", appPath); err != nil {
		return err
	}
	if err := runCommand("spctl", "-a", "-v", appPath); err != nil {
		return err
	}
	newZipPath, err := makeAppZip(appPath)
	if err != nil {
		return err
	}
	// Replace original zip with stapled one
	if err := os.Rename(newZipPath, zipFile); err != nil {
		return err
	}
	return nil
}

func findPrimaryBundleID(zipFile string) (string, error) {
	zr, err := zip.OpenReader(zipFile)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	for _, f := range zr.File {
		parts := strings.Split(f.Name, "/")
		if len(parts) == 3 &&
			filepath.Ext(parts[0]) == ".app" &&
			parts[1] == "Contents" &&
			parts[2] == "Info.plist" {

			ff, err := f.Open()
			if err != nil {
				return "", err
			}
			defer ff.Close()
			data, err := ioutil.ReadAll(ff)
			if err != nil {
				return "", err
			}
			var m map[string]interface{}
			dec := plist.NewDecoder(bytes.NewReader(data))
			if err := dec.Decode(&m); err != nil {
				return "", err
			}
			bundleID, ok := m["CFBundleIdentifier"].(string)
			if !ok {
				return "", errors.New("Info.plist does not contain a CFBundleIdentifier")
			}
			return bundleID, nil
		}
	}
	return "", errors.New("could not find Info.plist")
}

func submitForNotarization(zipFile, username, password string) (string, error) {
	bundleID, err := findPrimaryBundleID(zipFile)
	if err != nil {
		return "", err
	}
	fmt.Printf("submitting %s for notarization...\n", filepath.Base(zipFile))
	var buf bytes.Buffer
	args := []string{"xcrun", "altool",
		"--notarize-app",
		"--primary-bundle-id", bundleID,
		"--username", username,
		"--password", password,
		"--file", zipFile}

	if *verbose > 0 {
		args = append(args, "--verbose")
	}
	if err := writeCommandOutput(&buf, args...); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return "", err
		}
	}

	m := uuidRe.FindStringSubmatch(buf.String())
	if len(m) == 0 {
		m = uuidAltRe.FindStringSubmatch(buf.String())
		if len(m) == 0 {
			return "", errors.New("can't find RequestUUID in notarization response")
		}
	}

	return m[1], nil
}

func notarizationInfo(uuid, username, password string) (string, error) {
	var buf bytes.Buffer
	args := []string{"xcrun", "altool",
		"--notarization-info", uuid,
		"--username", username,
		"--password", password}
	if *verbose > 0 {
		args = append(args, "--verbose")
	}

	if err := writeCommandOutput(&buf, args...); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func waitForNotarization(uuid, username, password string) error {
	retryInterval := 10 * time.Second
	for {
		info, err := notarizationInfo(uuid, username, password)
		if err != nil {
			return err
		}
		m := statusRe.FindStringSubmatch(info)
		if len(m) == 0 {
			return errUnexpectedFormat
		}
		switch m[1] {
		case "success":
			fmt.Printf("notarization completed\n")
			return nil
		case "in progress":
			fmt.Printf("notarization in progress, will check again in %s...\n", retryInterval)
		case "invalid":
			if m := logFileURLRe.FindStringSubmatch(info); len(m) > 0 {
				resp, err := http.Get(m[1])
				if err != nil {
					errPrintf("error reading log: %v\n", err)
				} else {
					defer resp.Body.Close()
					io.Copy(os.Stderr, resp.Body)
					fmt.Fprint(os.Stderr, "\n")
				}
			} else {
				errPrintf("could not find log URL\n")
			}
			return errors.New("app notarization failed")
		default:
			return fmt.Errorf("unknown status %q", m[1])
		}
		time.Sleep(retryInterval)
	}
}

func notarizeZip(req notarizationRequest) error {
	var err error
	if req.UUID == "" {
		req.UUID, err = submitForNotarization(req.AppPath, req.Username, req.Password)
		if err != nil {
			return err
		}
	}
	fmt.Printf("waiting for notarization of %s\n", req.UUID)
	if err := waitForNotarization(req.UUID, req.Username, req.Password); err != nil {
		return err
	}
	if err := staple(req.AppPath); err != nil {
		return err
	}
	return nil
}

func unzipApp(appZip string, outputDir string) (string, error) {
	abs, err := filepath.Abs(appZip)
	if err != nil {
		return "", err
	}
	if err := runCommandOnDir(outputDir, "unzip", abs); err != nil {
		return "", err
	}
	entries, err := ioutil.ReadDir(outputDir)
	if err != nil {
		return "", err
	}
	for _, v := range entries {
		name := v.Name()
		if filepath.Ext(name) == ".app" {
			fullPath := filepath.Join(outputDir, name)
			if st, err := os.Stat(fullPath); err == nil && st.IsDir() {
				return fullPath, nil
			}
		}
	}
	return "", fmt.Errorf("couldn't find any .app directories at %s", outputDir)
}

func makeAppZip(appDir string) (string, error) {
	basename := filepath.Base(appDir)
	ext := filepath.Ext(basename)
	nonExt := basename[:len(basename)-len(ext)]
	zipFile := nonExt + ".zip"
	dir := filepath.Dir(appDir)
	fmt.Printf("compressing %s to %s\n",
		filepath.Join(dir, basename), filepath.Join(dir, zipFile))

	if err := runCommandOnDir(dir, "zip", "-9", "-y", "-r", zipFile, basename); err != nil {
		return "", err
	}
	return filepath.Join(dir, zipFile), nil
}

func notarizeFile(req notarizationRequest) error {
	if req.Username == "" {
		return errors.New("missing username")
	}
	if req.Password == "" {
		fmt.Printf("Password:")
		passwordData, err := terminal.ReadPassword(0)
		if err != nil {
			return err
		}
		req.Password = string(passwordData)
	}
	ext := filepath.Ext(req.AppPath)
	switch ext {
	case ".zip":
		return notarizeZip(req)
	case ".app":
		appZip, err := makeAppZip(req.AppPath)
		if err != nil {
			return err
		}
		req.AppPath = appZip
		return notarizeZip(req)
	default:
		return fmt.Errorf("can't notarize app in %s format", ext)
	}
}

type notarizeCmd struct {
	Username string
	Password string
	UUID     string
}

func (*notarizeCmd) Name() string {
	return "notarize"
}

func (*notarizeCmd) Synopsis() string {
	return "Notarize an app bundle"
}

func (*notarizeCmd) Usage() string {
	return `notarize [-u username][-p password] some.app`
}

func (c *notarizeCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if f.NArg() != 1 {
		return subcommands.ExitUsageError
	}
	var err error
	if c.Username == "" {
		prompt := promptui.Prompt{
			Label: "Username",
			Validate: func(s string) error {
				if s == "" {
					return errors.New("username can't be empty")
				}
				return nil
			},
		}
		c.Username, err = prompt.Run()
		if err != nil {
			errPrint(err)
			return subcommands.ExitFailure
		}
	}
	if c.Password == "" {
		pwPrompt := promptui.Prompt{
			Label: "Password",
			Validate: func(s string) error {
				if s == "" {
					return errors.New("password can't be empty")
				}
				return nil
			},
			Mask: '*',
		}
		c.Password, err = pwPrompt.Run()
		if err != nil {
			errPrint(err)
			return subcommands.ExitFailure
		}
	}
	app := f.Args()[0]
	if err := c.notarizeApp(app); err != nil {
		errPrintf("error notarizing %s: %v\n", app, err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

func (c *notarizeCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.Username, "u", "", "Apple Developer account username")
	f.StringVar(&c.Password, "p", "", "Apple Developer account application password")
	f.StringVar(&c.UUID, "uuid", "", "Already submitted UUID for notarization, used for checking the status of a previously submitted request")
}

func (c *notarizeCmd) notarizeApp(p string) error {
	req := notarizationRequest{
		AppPath:  p,
		Username: c.Username,
		Password: c.Password,
		UUID:     c.UUID,
	}
	return notarizeFile(req)
}
