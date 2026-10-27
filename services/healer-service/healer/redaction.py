"""Output-only credential masking; source snapshots and reviewed diffs stay intact."""
from collections import Counter
import math
import os
import re
from urllib.parse import urlparse

REDACTED = "[REDACTED]"
KEY = r'''(?:["']?)(?:[a-z0-9_.-]{0,64}(?:password|passwd|pwd|secret|token|api[_-]?key|access[_-]?key|credential)[a-z0-9_.-]{0,64}|authorization)(?:["']?)\s*[:=]\s*'''
_SENSITIVE = re.compile(KEY, re.IGNORECASE)
_BLOCK = re.compile(r"^([ \t]*)(?:" + KEY + r")[|>][+-]?[ \t]*$", re.IGNORECASE)
_PRIVATE = re.compile(r"-----BEGIN (?:[A-Z0-9]+ )?PRIVATE KEY-----.*?(?:-----END (?:[A-Z0-9]+ )?PRIVATE KEY-----|\Z)", re.DOTALL)
_PREFIX = re.compile(r"\b(?:gh[pousr]_[A-Za-z0-9_]{12,}|github_pat_[A-Za-z0-9_]{12,}|sk-(?:proj-)?[A-Za-z0-9_-]{12,}|xox[baprs]-[A-Za-z0-9-]{10,}|(?:AKIA|ASIA)[A-Z0-9]{16}|AIza[0-9A-Za-z_-]{20,})\b")
_JWT = re.compile(r"\beyJ[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b")
_BEARER = re.compile(r"\b(?:bearer|basic)\s+[A-Za-z0-9_+/=.-]+", re.IGNORECASE)
_URL = re.compile(r"\b([a-zA-Z][a-zA-Z0-9+.-]*://)[^\s/@:]+:[^\s/@]+@")
_ENTROPY = re.compile(r"[A-Za-z0-9_+/=-]{24,}")
_MANIFEST = re.compile(r"^[a-fA-F0-9]{64}\s+\*?\S+$")
_HEX = re.compile(r"^[a-fA-F0-9]+$")


def entropy(value: str) -> float:
    return -sum((count / len(value)) * math.log2(count / len(value)) for count in Counter(value).values()) if value else 0


def looks_secret(value: str) -> bool:
    if value == REDACTED or value.startswith('/'):
        return False
    if _HEX.fullmatch(value):
        return len(value) >= 48 and entropy(value) >= 3.5
    return (any(c.islower() for c in value) and any(c.isupper() for c in value) and any(c.isdigit() for c in value)
            and (len(value) >= 32 or any(not c.isalnum() for c in value)) and entropy(value) >= 4.3)


def known_secrets() -> list[str]:
    values = set()
    for name, value in os.environ.items():
        if _SENSITIVE.search(name + '=') and len(value) >= 4:
            values.add(value)
        try:
            password = urlparse(value).password
            if password and len(password) >= 4:
                values.add(password)
        except ValueError:
            continue
    return sorted(values, key=len, reverse=True)


def mask_output(text: str) -> str:
    lines = text.split("\n")
    index = 0
    while index < len(lines):
        block = _BLOCK.fullmatch(lines[index])
        if block:
            indent = len(block[1].expandtabs(8))
            index += 1
            while index < len(lines):
                content = lines[index]
                whitespace = content[:len(content) - len(content.lstrip(" \t"))]
                if content.strip() and len(whitespace.expandtabs(8)) <= indent:
                    break
                if content.strip():
                    lines[index] = whitespace + REDACTED
                index += 1
        else:
            index += 1
    text = _PRIVATE.sub(REDACTED, "\n".join(lines))
    for secret in known_secrets():
        text = text.replace(secret, REDACTED)
    # Consume multiline quoted assignments, including incomplete values at EOF.
    offset = 0
    for _ in range(64):
        match = _SENSITIVE.search(text, offset)
        if not match:
            break
        start = match.end()
        if text.startswith(REDACTED, start):
            offset = start + len(REDACTED)
            continue
        quote = text[start:start+1]
        if quote in {"'", '"'}:
            delimiter = quote * 3 if text.startswith(quote * 3, start) else quote
            end = start + len(delimiter)
            while True:
                close = text.find(delimiter, end)
                if close < 0 or close == 0 or text[close-1] != '\\':
                    break
                end = close + len(delimiter)
            if close < 0:
                text = text[:start] + quote + REDACTED
                break
            text = text[:start] + delimiter + REDACTED + text[close:]
            offset = start + len(delimiter) * 2 + len(REDACTED)
        else:
            end = text.find('\n', start)
            end = len(text) if end < 0 else end
            text = text[:start] + REDACTED + text[end:]
            offset = start + len(REDACTED)
    else:
        text = text[:offset] + REDACTED
    text = _BEARER.sub(REDACTED, _JWT.sub(REDACTED, _PREFIX.sub(REDACTED, text)))
    text = _URL.sub(lambda match: match[1] + REDACTED + '@', text)
    return '\n'.join(line if _MANIFEST.fullmatch(line.strip()) else _ENTROPY.sub(lambda match: REDACTED if looks_secret(match[0]) else match[0], line) for line in text.split('\n'))
