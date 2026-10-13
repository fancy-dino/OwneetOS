#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Claude Code hooks of this repository (.claude/settings.json), used when Claude Code is started in
# this folder. They are the knowledge base's own hooks (guards, reminder, re-injection after
# compaction, indexes), run only when the knowledge base sits next to this repository, as in
# docs/setup.md. Without it nothing is enforced: the session starts with a warning, and every
# other hook does nothing.
#
#   .claude/kb-hook.sh start         SessionStart: rebuilds the indexes and, when the workspace's
#                                    CLAUDE.md does not load them, loads the guards and the vault
#                                    rules; or warns that the knowledge base is missing
#   .claude/kb-hook.sh EVENT         guard-check, remind, reinject, indexes: passed to the knowledge
#                                    base's scripts/hook.sh

case "$0" in
    */*) HERE="${0%/*}" ;;
    *) HERE=. ;;
esac
KB_HOOK="$HERE/../../OwneetOS-knowledge-base/scripts/hook.sh"
EVENT="${1:-}"

if [ -f "$KB_HOOK" ]; then
    if [ "$EVENT" = start ]; then
        sh "$KB_HOOK" indexes
        # The guards reach the session through the workspace's CLAUDE.md (docs/setup.md, step 3.2).
        # Without it, they are loaded here instead: a CLAUDE.md cannot import conditionally.
        if ! grep -qs '^@OwneetOS-knowledge-base/CLAUDE\.md' "$HERE/../../CLAUDE.md"; then
            echo "The workspace's CLAUDE.md (../CLAUDE.md) does not load the OwneetOS knowledge base,"
            echo "so its guards and rules are loaded here. They are binding for this whole session."
            echo
            sh "$KB_HOOK" reinject
            echo
            cat "$HERE/../../OwneetOS-knowledge-base/kb-rules.md"
        fi
        exit 0
    fi
    exec sh "$KB_HOOK" "$EVENT"
fi

if [ "$EVENT" = start ]; then
    MSG="OwneetOS knowledge base not found next to this repository (../OwneetOS-knowledge-base):"
    MSG="$MSG its guards and hooks are NOT in use in this session. To use them, see docs/setup.md."
    printf '{"systemMessage": "%s", "hookSpecificOutput": {"hookEventName": "SessionStart", "additionalContext": "%s Tell the user so at the start of your first answer."}}\n' \
        "$MSG" "$MSG"
fi
exit 0
