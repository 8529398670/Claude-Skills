# whole-file feature:shortcuts
"""Keyboard shortcuts, kept per account so the keys someone picked follow them
to every browser. Only the ones they changed are stored: an action with no
entry uses its default, which lives in static/js/shortcuts.js with the action
itself.

A binding is OS-neutral: "Mod+Shift+Z" is Cmd+Shift+Z on a Mac and
Ctrl+Shift+Z elsewhere, so one saved choice works on every machine. An empty
binding switches the action's key off.

The server does not know the list of actions. It checks the shape of what it
is given, so an app that adds its own shortcut needs no change here. Two
actions may share keys when they never apply at the same time (Esc cancels an
edit and closes a dialog), so clashes are the frontend's to catch: it knows
where each action applies."""
from __future__ import annotations

import re

from app.core.db import BUCKET_USERS, Record, Store

MAX_SHORTCUTS = 40

_ACTION = re.compile(r"[a-z][a-z0-9_]{0,31}")
# A named key, as KeyboardEvent.key spells it: Enter, Escape, ArrowUp, F5,
# Space (the frontend's name for " ").
_NAMED_KEY = re.compile(r"[A-Z][A-Za-z0-9]{1,15}")
_MODIFIERS = ("Mod", "Ctrl", "Alt", "Shift")


class ShortcutsError(ValueError):
    """What is wrong with a set of shortcuts, worded for the caller."""


def valid_binding(binding: str) -> bool:
    """Modifiers, each at most once and in the order Mod, Ctrl, Alt, Shift,
    then one key -- a single character or a named key. The frontend writes
    bindings in exactly this form, so two spellings of the same keys can never
    both be stored."""
    if binding == "":
        return True
    rest = binding
    after = 0
    while True:
        for index in range(after, len(_MODIFIERS)):
            prefix = _MODIFIERS[index] + "+"
            # "Shift++" is Shift and the + key, so a prefix only counts when
            # something follows it.
            if rest.startswith(prefix) and len(rest) > len(prefix):
                rest = rest[len(prefix):]
                after = index + 1
                break
        else:
            break
    if len(rest) == 1:
        return rest > " " and rest != "\x7f"
    return _NAMED_KEY.fullmatch(rest) is not None


def check(shortcuts: object) -> dict[str, str]:
    """The set, checked before it replaces the stored one. ShortcutsError if
    it cannot be stored."""
    if not isinstance(shortcuts, dict):
        raise ShortcutsError("shortcuts must be an object")
    if len(shortcuts) > MAX_SHORTCUTS:
        raise ShortcutsError(f"at most {MAX_SHORTCUTS} shortcuts")
    for action, binding in shortcuts.items():
        if not isinstance(action, str) or _ACTION.fullmatch(action) is None:
            raise ShortcutsError(f"unknown shortcut name {action!r}")
        if not isinstance(binding, str) or not valid_binding(binding):
            raise ShortcutsError(f"{action}: not a key the app can use")
    return dict(shortcuts)


def set_shortcuts(store: Store, user_id: int, shortcuts: dict[str, str]) -> bool:
    """Replaces the account's changed shortcuts. An empty dict puts every one
    back to its default. False if there is no such account."""
    checked = check(shortcuts)

    def mutate(record: Record) -> None:
        if checked:
            record["shortcuts"] = checked
        else:
            record.pop("shortcuts", None)

    return store.update_value(BUCKET_USERS, user_id, mutate) is not None
