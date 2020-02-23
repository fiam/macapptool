package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func osRemove(path string) error {
	if *dryRun {
		fmt.Printf("rm %s\n", path)
		return nil
	}
	return os.Remove(path)
}

func osRename(source, dest string) error {
	if *dryRun {
		fmt.Printf("mv %s %s\n", source, dest)
		return nil
	}
	return os.Rename(source, dest)
}

func symlink(source, dest string) error {
	if *dryRun {
		fmt.Printf("ln -s %s %s\n", dest, source)
		return nil
	}
	symRel, err := filepath.Rel(filepath.Dir(source), dest)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(filepath.Dir(source)); err != nil {
		return err
	}
	defer os.Chdir(cwd)
	if err := os.Symlink(symRel, filepath.Base(symRel)); err != nil {
		return err
	}
	return nil

}
