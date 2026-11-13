# whole-file feature:passwords
"""Password sign-in (app/models/passwords.py). None of this exists while
auth.passwords is off -- RequirePasswords answers 404 -- except
/api/login/options, which is how the signed-out page finds out.

Passwords never reach these routes: password_digest is what the browser
derived from one (static/js/password.js).
"""
from __future__ import annotations

from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from app.core.encryption import BadKeyError, CipherTextError
from app.core.logs import KIND_ADMIN, KIND_AUTH, LEVEL_INFO, LEVEL_WARN, anonymous
from app.core.middleware import client_ip, trusted_for
from app.core.security import actor_from, check_csrf, decrypt_cookie
from app.models import passwords
from app.models import two_factor  # feature:two_factor
from app.models.attempts import AttemptGuard
from app.models.sessions import split_credential
from app.routes.auth import same_origin_json, sign_in_response, sign_in_rules
from app.routes.helpers import bad_request, conflict, error, forbidden, get_config, get_store, not_found, parse_id, read_json, run, server_error
from app.routes.logs import audit, audit_as, user_actor
from app.routes.two_factor import CODES  # feature:two_factor

# Per name typed, not per account, so a lockout does not say which names are
# real. A wrong guess from one address across many names is what the second
# one is for: every attempt costs a bcrypt. Per process, like the rate limiter.
NAMES = AttemptGuard()
ADDRESSES = AttemptGuard(max_failures=30)


async def login_options(request: Request) -> Response:
    """What the signed-out page needs to know. Public, and the same for
    everyone: the salt is not a secret, and there is deliberately nothing here
    that depends on a username."""
    cfg = get_config(request)
    payload = {"passwords": cfg.passwords_enabled, "two_factor": cfg.two_factor_enabled}  # feature:two_factor
    payload = {"passwords": cfg.passwords_enabled, "two_factor": False}  # feature:!two_factor
    if cfg.passwords_enabled:
        payload["salt"] = await run(passwords.install_salt, get_store(request))
    return JSONResponse(payload)


async def password_login(request: Request) -> Response:
    """Unknown name, wrong password, a blocked or disabled account: all the
    same answer, after the same bcrypt. No CSRF token, for the same reason as
    /api/login."""
    if not same_origin_json(request):
        return forbidden("login must be submitted from this site's own login page")
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")

    store, cfg = get_store(request), get_config(request)
    name = passwords.normalize_username(body.get("username"))[:64]
    address = client_ip(request.scope, cfg, trusted_for(cfg))
    # Reserved before the password is checked, so guesses sent in parallel
    # cannot all get past the lockout (see app/models/attempts.py).
    by_name = NAMES.reserve(name)
    if by_name is None:
        return error(429, "login_locked")
    with by_name:
        by_address = ADDRESSES.reserve(address)
        if by_address is None:
            return error(429, "login_locked")
        with by_address:
            user = await run(passwords.check_password, store, name, body.get("password_digest"))
            signed = None if user is None else await run(passwords.start_password_sign_in, store, user, sign_in_rules(cfg))
            if signed is None:
                by_name.failed()
                by_address.failed()
                audit_as(request, anonymous(), KIND_AUTH, LEVEL_WARN, "password refused")
                return bad_request("invalid_login")
            by_name.passed()
    message = "password accepted, waiting for a code" if signed.challenge else "signed in with a password"  # feature:two_factor
    message = "signed in with a password"  # feature:!two_factor
    audit_as(request, await run(user_actor, store, user.id, "session"), KIND_AUTH, LEVEL_INFO, message)
    return await sign_in_response(request, signed, {"ok": True}, 200)


def _session_id(request: Request) -> str:
    """The id of the session this request came with, to leave it alone when
    the account's others end."""
    cfg = get_config(request)
    try:
        parts = split_credential(decrypt_cookie(cfg, request.cookies.get(cfg.cookie_name, "")))
    except (CipherTextError, BadKeyError):
        return ""
    return "" if parts is None else parts[0]


def _set_password_refused(refusal: Exception) -> Response:
    if isinstance(refusal, passwords.BadUsernameError):
        return bad_request("bad_username")
    if isinstance(refusal, passwords.BadDigestError):
        return bad_request("bad_digest")
    if isinstance(refusal, passwords.UsernameTakenError):
        return conflict("username_taken")
    return forbidden("password_blocked")


async def set_password(request: Request) -> Response:
    """Set the account's username and password, or with an empty
    password_digest remove the password. Session only: a key that could set
    one could turn itself into a session."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")

    store, cfg = get_store(request), get_config(request)
    user = actor_from(request).user
    digest = body.get("password_digest")
    # >>> feature:two_factor
    # Reserved before the code is checked (see app/models/attempts.py). A
    # right code here forgives nothing, as before: only a wrong one counts.
    attempt = CODES.reserve(user.id)
    if attempt is None:
        return error(429, "two_factor_locked")
    with attempt:
        try:
            found = await run(
                passwords.set_password, store, user.id, body.get("username"), digest, _session_id(request),
                body.get("code"), cfg.two_factor_enabled, cfg.secret_key,
            )
        except two_factor.WrongCodeError:
            attempt.failed()
            return forbidden("two_factor_wrong")
        except (passwords.BadUsernameError, passwords.BadDigestError, passwords.UsernameTakenError, passwords.PasswordBlockedError) as refusal:
            return _set_password_refused(refusal)
    # <<< feature:two_factor
    # >>> feature:!two_factor
    try:
        found = await run(passwords.set_password, store, user.id, body.get("username"), digest, _session_id(request))
    except (passwords.BadUsernameError, passwords.BadDigestError, passwords.UsernameTakenError, passwords.PasswordBlockedError) as refusal:
        return _set_password_refused(refusal)
    # <<< feature:!two_factor
    if not found:
        return server_error(request, "set password: the signed-in account has no record")
    audit(request, KIND_AUTH, "password removed" if digest == "" else "password changed")
    return JSONResponse({"ok": True, "has_password": digest != ""})


async def set_password_access(request: Request) -> Response:
    """The owner's per-account switch (RequireOwner). {allowed: false} stops
    this account's password working and stops it setting one."""
    body = await read_json(request)
    if body is None:
        return bad_request("malformed request body")
    if not check_csrf(request, body.get("csrf_token")):
        return forbidden("invalid csrf token")
    user_id = parse_id(request.path_params["user_id"])
    if user_id is None:
        return bad_request("invalid user id")
    allowed = body.get("allowed")
    if not isinstance(allowed, bool):
        return bad_request("allowed must be true or false")
    if not await run(passwords.set_blocked, get_store(request), user_id, not allowed):
        return not_found("no such user")
    audit(request, KIND_ADMIN, "password access set", {"user_id": user_id, "allowed": allowed})
    return JSONResponse({"ok": True, "password_blocked": not allowed})
