package phpstrom

import "github.com/ayanozturk/vscode-php-strom/providers"

func enableStyleAnalysis(h *Handler) {
	h.cfg.Diagnostics.Analysis.Style = true
	h.prov = providers.NewRegistry(h.idx, h.cfg.toProviderConfig(h.idx.WorkspaceFolders()))
}
