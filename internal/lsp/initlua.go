package lsp

import (
	"fmt"
	"os"
	"strings"
)

// WriteInitLua writes a temporary init.lua that configures neovim to connect
// to the qry LSP server at the given unix socket path.
func WriteInitLua(sockPath string) (string, error) {
	lua := strings.ReplaceAll(initLuaTemplate, "{{sockPath}}", sockPath)

	f, err := os.CreateTemp("", "qry-lsp-*.lua")
	if err != nil {
		return "", fmt.Errorf("create init.lua: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(lua); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("write init.lua: %w", err)
	}
	return f.Name(), nil
}

const initLuaTemplate = `-- qry LSP: attach after buffer loads
local function start_qry_lsp()
  vim.lsp.start({
    name = 'qry',
    cmd = vim.lsp.rpc.connect('{{sockPath}}'),
    root_dir = vim.fn.getcwd(),
  })
  vim.bo.omnifunc = 'v:lua.vim.lsp.omnifunc'
end

-- Attach to any SQL buffer that opens (covers the initial file too)
vim.api.nvim_create_autocmd({'BufReadPost', 'FileType'}, {
  pattern = {'*.sql', 'sql'},
  callback = function()
    -- small delay so filetype detection finishes
    vim.schedule(start_qry_lsp)
  end,
})
`
