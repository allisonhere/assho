package main

import (
	"fmt"
	"io"
)

func fprintAliases(w io.Writer, hosts []Host) {
	for _, h := range hosts {
		if !h.IsContainer {
			fmt.Fprintln(w, h.Alias)
		}
		for _, c := range h.Containers {
			fmt.Fprintln(w, c.Alias)
		}
	}
}

const bashCompletion = `# bash completion for assho
# Add to ~/.bashrc:  eval "$(assho completion bash)"
_assho_completions() {
    local cur="${COMP_WORDS[COMP_CWORD]}"
    case "${COMP_WORDS[1]}" in
        connect|test)
            # shellcheck disable=SC2207
            COMPREPLY=($(compgen -W "$(assho _aliases 2>/dev/null)" -- "$cur"))
            ;;
        completion)
            COMPREPLY=($(compgen -W "bash zsh fish" -- "$cur"))
            ;;
        *)
            COMPREPLY=($(compgen -W "connect test list export completion --version" -- "$cur"))
            ;;
    esac
}
complete -F _assho_completions assho`

const zshCompletion = `# zsh completion for assho
# Add to ~/.zshrc:  eval "$(assho completion zsh)"
_assho() {
    local -a subcmds
    subcmds=(
        'connect:connect to a host by alias'
        'test:test SSH connectivity for an alias'
        'list:list all configured hosts'
        'export:print hosts as SSH config stanzas'
        'completion:generate shell completion scripts'
        '--version:print version and exit'
    )

    if (( CURRENT == 2 )); then
        _describe 'command' subcmds
        return
    fi

    case "${words[2]}" in
        connect|test)
            local -a aliases
            aliases=(${(f)"$(assho _aliases 2>/dev/null)"})
            _describe 'alias' aliases
            ;;
        completion)
            local -a shells
            shells=(bash zsh fish)
            _describe 'shell' shells
            ;;
    esac
}
compdef _assho assho`

const fishCompletion = `# fish completion for assho
# Install: assho completion fish > ~/.config/fish/completions/assho.fish
function __assho_no_subcommand
    not __fish_seen_subcommand_from connect test list completion --version
end

complete -c assho -f
complete -c assho -n '__assho_no_subcommand' -a connect    -d 'Connect to a host'
complete -c assho -n '__assho_no_subcommand' -a test       -d 'Test SSH connectivity'
complete -c assho -n '__assho_no_subcommand' -a list       -d 'List all hosts'
complete -c assho -n '__assho_no_subcommand' -a export     -d 'Print hosts as SSH config stanzas'
complete -c assho -n '__assho_no_subcommand' -a completion -d 'Generate shell completions'
complete -c assho -n '__assho_no_subcommand' -a --version  -d 'Print version'
complete -c assho -n '__fish_seen_subcommand_from connect test' \
    -a '(assho _aliases 2>/dev/null)'`
