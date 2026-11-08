# whole-file feature:logs
"""What the owner calls an address or a range of them, so the Logs and Sessions
screens can say who is who. A name is applied when something reads the log: the
log itself keeps the address, so renaming one renames its past too. Go's twin is
server/models/ip_name.go.
"""
from __future__ import annotations

import ipaddress
from dataclasses import dataclass
from typing import Any

from app.core.db import BUCKET_IP_NAMES, Store
from app.models import to_iso, utc_now

NAME_MAX = 80


def clean_target(text: Any) -> str | None:
    """An address (10.0.0.5) or a range (10.0.0.0/24), said the one way it is
    stored: a range starts at its first address, and a range of exactly one
    address is that address. None if it is neither."""
    if not isinstance(text, str):
        return None
    text = text.strip()
    try:
        return str(_plain(ipaddress.ip_address(text)))
    except ValueError:
        pass
    try:
        network = ipaddress.ip_network(text, strict=False)
    except ValueError:
        return None
    return str(network.network_address) if network.num_addresses == 1 else str(network)


def _plain(address: Any) -> Any:
    """The IPv4 address inside an IPv4-mapped one, and no zone."""
    mapped = getattr(address, "ipv4_mapped", None)
    if mapped is not None:
        return mapped
    if getattr(address, "scope_id", None):
        return ipaddress.ip_address(str(address).partition("%")[0])
    return address


def valid_name(name: Any) -> bool:
    return isinstance(name, str) and 1 <= len(name.strip()) <= NAME_MAX


def set_name(store: Store, target: str, name: str) -> None:
    """Names a target, or renames it."""
    store.put(BUCKET_IP_NAMES, target, {"target": target, "name": name.strip(), "created_at": to_iso(utc_now())})


def delete_name(store: Store, target: str) -> None:
    store.delete(BUCKET_IP_NAMES, target)


def list_names(store: Store) -> list[dict[str, Any]]:
    """Every name, by what it is called."""
    return sorted((record for _, record in store.all(BUCKET_IP_NAMES)), key=lambda record: record["name"].lower())


@dataclass
class _Range:
    network: Any
    name: str


class Namer:
    """What an address is called, for one read of the log, which can ask about
    the same few addresses thousands of times. The most specific name wins: an
    exact address over any range, and a narrower range over a wider one."""

    def __init__(self, names: list[dict[str, Any]]) -> None:
        self.exact: dict[str, str] = {}
        self.ranges: list[_Range] = []
        self.seen: dict[str, str] = {}
        for item in names:
            if "/" in item["target"]:
                self.ranges.append(_Range(ipaddress.ip_network(item["target"]), item["name"]))
            else:
                self.exact[item["target"]] = item["name"]
        self.ranges.sort(key=lambda item: item.network.prefixlen, reverse=True)

    def __call__(self, ip: str) -> str:
        if not ip:
            return ""
        if ip not in self.seen:
            self.seen[ip] = self._find(ip)
        return self.seen[ip]

    def _find(self, ip: str) -> str:
        try:
            address = _plain(ipaddress.ip_address(ip))
        except ValueError:
            return ""
        found = self.exact.get(str(address))
        if found is not None:
            return found
        for item in self.ranges:
            if address.version == item.network.version and address in item.network:
                return item.name
        return ""


def load_namer(store: Store) -> Namer:
    return Namer(list_names(store))
