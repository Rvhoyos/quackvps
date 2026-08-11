// Package picker is the interactive directory browser used during the wizard.
// It's the one visual step that isn't a plain huh form: a keyboard-driven tree of
// directories. Two entry points, because the target differs by flow — install
// picks the PARENT container that will hold servers, update picks an existing
// server INSTANCE inside it. Like the rest of the wizard, it only gathers input;
// it performs no install work (the one exception is creating a folder the user
// explicitly asks for).
package picker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Digital-Shane/treeview"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// PickParent lets the user choose the container folder that holds servers, then
// optionally create a fresh subfolder inside it (so a bare VPS without ~/mcserver
// is never a dead end). Returns the chosen parent directory.
func PickParent(ctx context.Context, start string) (string, error) {
	chosen, err := browse(ctx, start, "Choose the folder that will hold your servers (e.g. ~/mcserver).")
	if err != nil {
		return "", err
	}
	return maybeCreateSubfolder(chosen)
}

// PickInstance lets the user choose an existing server subfolder inside parent to
// update. Returns the selected instance directory.
func PickInstance(ctx context.Context, parent string) (string, error) {
	return browse(ctx, parent, "Choose the server folder to update.")
}

// maybeCreateSubfolder offers to create a new subfolder inside chosen. This is the
// picker's "create new folder here" affordance; declining keeps chosen as-is.
func maybeCreateSubfolder(chosen string) (string, error) {
	create := false
	prompt := huh.NewConfirm().
		Title(fmt.Sprintf("Create a new folder inside %s?", chosen)).
		Description("Pick Yes on a fresh VPS to make the container (e.g. mcserver); No to use this folder as-is.").
		Value(&create)
	if err := prompt.Run(); err != nil {
		return "", err
	}
	if !create {
		return chosen, nil
	}

	name := "mcserver"
	nameField := huh.NewInput().
		Title("New folder name").
		Description("This becomes the container that holds each server instance.").
		Value(&name).
		Validate(validFolderName)
	if err := nameField.Run(); err != nil {
		return "", err
	}
	dir := filepath.Join(chosen, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	return dir, nil
}

func validFolderName(name string) error {
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if filepath.Base(name) != name {
		return fmt.Errorf("use a plain folder name, not a path")
	}
	return nil
}

// browse runs the directory tree and returns the focused directory's path.
func browse(ctx context.Context, start, help string) (string, error) {
	tree, err := treeview.NewTreeFromFileSystem(ctx, start, false,
		// Directories only, and skip hidden ones (.ssh, .cache, …) — they're never
		// a place to install a server and only clutter the list.
		treeview.WithFilterFunc(func(info treeview.FileInfo) bool {
			return info.IsDir() && !strings.HasPrefix(info.Name(), ".")
		}),
		treeview.WithExpandFunc(func(n *treeview.Node[treeview.FileInfo]) bool { return false }),
		treeview.WithMaxDepth[treeview.FileInfo](4),
	)
	if err != nil {
		return "", fmt.Errorf("read directories under %s: %w", start, err)
	}

	// Disable the library's search-on-Enter so our wrapper can use Enter to
	// accept; navigation stays on the arrow keys.
	km := treeview.DefaultKeyMap()
	km.SearchStart = nil
	km.SearchAccept = nil
	km.Quit = nil // our wrapper owns quitting (accept vs cancel)

	inner := treeview.NewTuiTreeModel(tree,
		treeview.WithTuiWidth[treeview.FileInfo](80),
		treeview.WithTuiHeight[treeview.FileInfo](24),
		treeview.WithTuiKeyMap[treeview.FileInfo](km),
	)

	m := &model{inner: inner, help: help}
	// Alt-screen isolates the picker on its own buffer so its frame doesn't linger
	// in the scrollback above the prompts that follow.
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		return "", fmt.Errorf("directory picker: %w", err)
	}
	if m.cancelled {
		return "", fmt.Errorf("directory selection cancelled")
	}
	node := tree.GetFocusedNode()
	if node == nil {
		return "", fmt.Errorf("no folder selected")
	}
	return node.Data().Path, nil
}
