# {{PROJECT_NAME}} API

Every route this server answers, apart from the pages at /login/* and /join/*. Generated from the code by scripts/api_docs.sh: an app's own routes get their docs the same way, and a test fails until they do.

This file is generated: do not edit it. Change the docs next to the handlers and run `scripts/api_docs.sh`.

## Contents

- [Calling the API](#calling-the-api)
- [Errors](#errors)
- [Account](#account)
- [Sign-in](#sign-in)
- [API keys](#api-keys)
- [Files](#files)
- [Admin](#admin)
- [Invite links](#invite-links)
- [Owner](#owner)
- [Webhooks](#webhooks)
- [Monitoring](#monitoring)
- [Schemas](#schemas)
- [MCP tools](#mcp-tools)

## Calling the API

| Way in | How |
|---|---|
| Browser session | A browser session. Its POSTs carry csrf_token in the body. The cookie's name is server.cookie_name. |
| API key | An API key, as Authorization: Bearer <id>.<secret>. |

## Errors

An error reply is a JSON object whose `error` says what went wrong.

| Status | Meaning | Body |
|---|---|---|
| 400 | The request was refused as it stands. | [`Error`](#error) |
| 401 | Not signed in. | [`Error`](#error) |
| 403 | Not allowed, or a bad csrf_token. | [`Error`](#error) |
| 404 | No such thing, or this feature is switched off. | [`Error`](#error) |
| 409 | The record has moved on: an undo too late, or a stale edit. | [`Error`](#error) |
| 429 | Too many requests. | [`Error`](#error) |
| 503 | In maintenance mode. Retry-After says when to try again. | [`Maintenance`](#maintenance) |

## Account

| Method | Path | Summary | Who may call |
|---|---|---|---|
| POST | [`/api/account/login-link`](#post-apiaccountlogin-link) | A login link for yourself | browser session or API key, anyone signed in |
| POST | [`/api/account/login-link/cancel`](#post-apiaccountlogin-linkcancel) | Stop your own login link working | browser session or API key, anyone signed in |
| POST | [`/api/account/password`](#post-apiaccountpassword) | Set or clear your password | browser session only, anyone signed in |
| POST | [`/api/account/pin`](#post-apiaccountpin) | Set or clear your PIN | browser session only, anyone signed in |
| POST | [`/api/account/rename`](#post-apiaccountrename) | Change your display name | browser session or API key, anyone signed in |
| GET | [`/api/account/sessions`](#get-apiaccountsessions) | Your signed-in devices | browser session or API key, anyone signed in |
| POST | [`/api/account/sessions/revoke-others`](#post-apiaccountsessionsrevoke-others) | Sign out every other device | browser session or API key, anyone signed in |
| POST | [`/api/account/sessions/{session_id}/label`](#post-apiaccountsessionssession_idlabel) | Name one of your devices | browser session or API key, anyone signed in |
| POST | [`/api/account/sessions/{session_id}/revoke`](#post-apiaccountsessionssession_idrevoke) | Sign out one device | browser session or API key, anyone signed in |
| POST | [`/api/account/shortcuts`](#post-apiaccountshortcuts) | Change your keyboard shortcuts | browser session or API key, anyone signed in |
| POST | [`/api/account/theme`](#post-apiaccounttheme) | Pick which theme you use | browser session or API key, anyone signed in |
| POST | [`/api/account/themes`](#post-apiaccountthemes) | Replace your own themes | browser session or API key, anyone signed in |
| POST | [`/api/account/two-factor/confirm`](#post-apiaccounttwo-factorconfirm) | Turn two-factor on | browser session only, anyone signed in |
| POST | [`/api/account/two-factor/disable`](#post-apiaccounttwo-factordisable) | Turn two-factor off | browser session only, anyone signed in |
| POST | [`/api/account/two-factor/start`](#post-apiaccounttwo-factorstart) | Begin two-factor setup | browser session only, anyone signed in |
| GET | [`/api/language`](#get-apilanguage) | The UI's text | anyone |
| POST | [`/api/logout`](#post-apilogout) | Sign out | browser session or API key, anyone signed in |
| POST | [`/api/logs/events`](#post-apilogsevents) | Record what happened on the page | browser session or API key, anyone signed in |
| GET | [`/api/me`](#get-apime) | Who am I | browser session or API key, anyone signed in |
| GET | [`/api/team`](#get-apiteam) | Everyone's names | browser session or API key, anyone signed in |

### POST /api/account/login-link

A login link for yourself

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`create_my_login_link`](#create_my_login_link) |
| Errors | 400, 401, 403, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `expires_in_minutes` | integer | no | 0 to 525600 | 0 never expires. Default auth.login_token_ttl_seconds for a link that works once, auth.multi_use_login_token_ttl_seconds otherwise. |
| `max_uses` | integer | no | 0 to 1000 | Sign-ins the link allows: 1-1000, or 0 for unlimited. Default 1. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `expires_at` | string (date-time) or null | yes |  | null when it never expires. |
| `expires_in_seconds` | integer | yes |  | 0 when it never expires. |
| `login_path` | string | yes |  | Path of the login link. A credential: treat it like a password. |
| `max_uses` | integer | yes |  | 0 when unlimited. |
| `unlimited` | boolean | yes |  |  |

### POST /api/account/login-link/cancel

Stop your own login link working

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`cancel_my_login_link`](#cancel_my_login_link) |
| Errors | 400, 401, 403, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |
| `removed` | integer | yes |  | Links removed: 0 or 1. |

### POST /api/account/password

Set or clear your password

|  |  |
|---|---|
| Who may call | browser session only, anyone signed in |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `code` | string | no |  |  |
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `password_digest` | string | yes |  |  |
| `username` | string | yes |  |  |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `has_password` | boolean | yes |  |  |
| `ok` | boolean | yes |  |  |

### POST /api/account/pin

Set or clear your PIN

|  |  |
|---|---|
| Who may call | browser session only, anyone signed in |
| Errors | 400, 401, 403, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `pin` | string | yes |  |  |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `has_pin` | boolean | yes |  |  |
| `ok` | boolean | yes |  |  |

### POST /api/account/rename

Change your display name

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`rename_account`](#rename_account) |
| Errors | 400, 401, 403, 409, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `display_name` | string | yes |  |  |
| `expect` | string or null | no |  |  |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `display_name` | string | yes |  |  |
| `ok` | boolean | yes |  |  |

### GET /api/account/sessions

Your signed-in devices

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`list_my_sessions`](#list_my_sessions) |
| Errors | 400, 401, 403, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `limit` | query | integer | no | 1 to 1000 | Rows to return. Default 200. |
| `offset` | query | integer | no | at least 0 |  |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `next` | integer or null | yes |  | Offset of the next page, or null on the last one. |
| `sessions` | array of [`Session`](#session) | yes |  |  |
| `total` | integer | yes |  |  |

### POST /api/account/sessions/revoke-others

Sign out every other device

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`revoke_my_other_sessions`](#revoke_my_other_sessions) |
| Errors | 400, 401, 403, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |
| `removed` | integer | yes |  |  |

### POST /api/account/sessions/{session_id}/label

Name one of your devices

This device included. Empty clears the name.

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`label_my_session`](#label_my_session) |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `session_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `expect` | string | no |  | The name you think it has. Answers 409 if it has another. |
| `label` | string | yes |  | At most 80 characters. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `label` | string | yes |  |  |
| `ok` | boolean | yes |  |  |

### POST /api/account/sessions/{session_id}/revoke

Sign out one device

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`revoke_my_session`](#revoke_my_session) |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `session_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### POST /api/account/shortcuts

Change your keyboard shortcuts

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`set_shortcuts`](#set_shortcuts) |
| Errors | 400, 401, 403, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `shortcuts` | map of string | yes |  | Action name to binding, such as "search": "Mod+K". Mod is Cmd on a Mac and Ctrl elsewhere. Only changed shortcuts are listed; an empty binding switches that key off. Replaces the whole set; {} puts every key back to its default. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |
| `shortcuts` | map of string | yes |  |  |

### POST /api/account/theme

Pick which theme you use

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`set_theme`](#set_theme) |
| Errors | 400, 401, 403, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `theme_id` | string | yes |  | A theme's id. Empty for the first in the list. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |
| `theme_id` | string | yes |  |  |

### POST /api/account/themes

Replace your own themes

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`set_themes`](#set_themes) |
| Errors | 400, 401, 403, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `theme_id` | string | no |  | The theme you use. |
| `themes` | array of [`Theme`](#theme) or null | no |  | Your whole list, 1 to 40 themes. null goes back to following the app's defaults. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |
| `theme_id` | string | yes |  |  |
| `themes` | array of [`Theme`](#theme) or null | yes |  | What you now see; null while the built-in themes apply. |
| `themes_custom` | boolean | yes |  |  |

### POST /api/account/two-factor/confirm

Turn two-factor on

|  |  |
|---|---|
| Who may call | browser session only, anyone signed in |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `code` | string | yes |  |  |
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `recovery_codes` | array of string | yes |  |  |

### POST /api/account/two-factor/disable

Turn two-factor off

|  |  |
|---|---|
| Who may call | browser session only, anyone signed in |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `code` | string | yes |  |  |
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### POST /api/account/two-factor/start

Begin two-factor setup

|  |  |
|---|---|
| Who may call | browser session only, anyone signed in |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `secret` | string | yes |  |  |

### GET /api/language

The UI's text

|  |  |
|---|---|
| Who may call | anyone |
| Errors | 429, 503 |

**Reply 200**: Success

An empty object.

### POST /api/logout

Sign out

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | None, HTTP only: Ends a browser session; a key has none to end. |
| Errors | 400, 401, 403, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### POST /api/logs/events

Record what happened on the page

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | None, HTTP only: The browser's own activity reports (track.js), not an action. |
| Errors | 400, 401, 403, 404, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `events` | array of object | yes |  |  |
| `tab` | string | yes |  |  |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `accepted` | integer | yes |  |  |

### GET /api/me

Who am I

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`whoami`](#whoami) |
| Errors | 401, 429, 503 |

**Reply 200**: Success

[`Me`](#me).

### GET /api/team

Everyone's names

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`list_team`](#list_team) |
| Errors | 400, 401, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `limit` | query | integer | no | 1 to 1000 | Rows to return. Default 200. |
| `offset` | query | integer | no | at least 0 |  |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `members` | array of [`Member`](#member) | yes |  |  |
| `next` | integer or null | yes |  | Offset of the next page, or null on the last one. |
| `total` | integer | yes |  |  |

## Sign-in

| Method | Path | Summary | Who may call |
|---|---|---|---|
| POST | [`/api/login`](#post-apilogin) | Redeem a login link | anyone |
| GET | [`/api/login/options`](#get-apiloginoptions) | What sign-in offers | anyone |
| POST | [`/api/login/password`](#post-apiloginpassword) | Sign in with a password | anyone |
| POST | [`/api/login/two-factor`](#post-apilogintwo-factor) | Finish a sign-in with a code | anyone |

### POST /api/login

Redeem a login link

|  |  |
|---|---|
| Who may call | anyone |
| Errors | 400, 403, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `credential` | string | yes |  |  |

**Reply 200**: Success

[`SignIn`](#signin).

### GET /api/login/options

What sign-in offers

|  |  |
|---|---|
| Who may call | anyone |
| Errors | 429, 503 |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `passwords` | boolean | yes |  |  |
| `salt` | string | no |  |  |
| `two_factor` | boolean | yes |  |  |

### POST /api/login/password

Sign in with a password

|  |  |
|---|---|
| Who may call | anyone |
| Errors | 400, 403, 404, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `password_digest` | string | yes |  |  |
| `username` | string | yes |  |  |

**Reply 200**: Success

[`SignIn`](#signin).

### POST /api/login/two-factor

Finish a sign-in with a code

|  |  |
|---|---|
| Who may call | anyone |
| Errors | 400, 403, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `challenge` | string | yes |  |  |
| `code` | string | yes |  |  |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

## API keys

| Method | Path | Summary | Who may call |
|---|---|---|---|
| GET | [`/api/admin/keys`](#get-apiadminkeys) | Every key in the app | browser session only, admins |
| POST | [`/api/admin/keys`](#post-apiadminkeys) | Create a key that acts as someone else | browser session only, admins |
| POST | [`/api/admin/keys/{key_id}/delete`](#post-apiadminkeyskey_iddelete) | Delete anyone's key | browser session only, admins |
| POST | [`/api/admin/keys/{key_id}/expiry`](#post-apiadminkeyskey_idexpiry) | Change when anyone's key expires | browser session only, admins |
| POST | [`/api/admin/keys/{key_id}/rename`](#post-apiadminkeyskey_idrename) | Rename anyone's key | browser session only, admins |
| POST | [`/api/admin/keys/{key_id}/restore`](#post-apiadminkeyskey_idrestore) | Undo deleting anyone's key | browser session only, admins |
| POST | [`/api/admin/keys/{key_id}/revoke`](#post-apiadminkeyskey_idrevoke) | Revoke anyone's key | browser session only, admins |
| GET | [`/api/keys`](#get-apikeys) | Your API keys | browser session only, anyone signed in |
| POST | [`/api/keys`](#post-apikeys) | Create an API key | browser session only, anyone signed in |
| POST | [`/api/keys/{key_id}/delete`](#post-apikeyskey_iddelete) | Delete one of your keys | browser session only, anyone signed in |
| POST | [`/api/keys/{key_id}/expiry`](#post-apikeyskey_idexpiry) | Change when one of your keys expires | browser session only, anyone signed in |
| POST | [`/api/keys/{key_id}/rename`](#post-apikeyskey_idrename) | Rename one of your keys | browser session only, anyone signed in |
| POST | [`/api/keys/{key_id}/restore`](#post-apikeyskey_idrestore) | Undo deleting one of your keys | browser session only, anyone signed in |
| POST | [`/api/keys/{key_id}/revoke`](#post-apikeyskey_idrevoke) | Revoke one of your keys | browser session only, anyone signed in |

### GET /api/admin/keys

Every key in the app

|  |  |
|---|---|
| Who may call | browser session only, admins |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `limit` | query | integer | no | 1 to 1000 | Rows to return. Default 200. |
| `offset` | query | integer | no | at least 0 |  |
| `q` | query | string | no |  | Keep keys whose name or owner contains this, in any case. |
| `live` | query | string | no | one of `true` | Leave out revoked and expired keys. |
| `sort` | query | string | no | one of `display_name`, `name`, `role`, `last_used_at`, `expires_at`, `created_at` | Field to sort the whole list by. Rows without it go last. |
| `dir` | query | string | no | one of `asc`, `desc` | Default asc. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `keys` | array of [`APIKey`](#apikey) | yes |  |  |
| `next` | integer or null | yes |  | Offset of the next page, or null on the last one. |
| `total` | integer | yes |  |  |

### POST /api/admin/keys

Create a key that acts as someone else

Any admin may do this for an ordinary user, and only the owner for another admin. Nobody may do it for the owner. The key records who made it and shows that to the person it acts as.

|  |  |
|---|---|
| Who may call | browser session only, admins |
| Errors | 400, 401, 403, 404, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `expires_in_days` | integer or null | no | 0 to 3650 | Absent or null: the configured default. 0: never expires. |
| `name` | string | yes | 1 to 80 characters |  |
| `role` | string | no | one of `admin`, `user`; default `user` |  |
| `user_id` | integer | yes |  |  |

**Reply 201**: Success

[`NewAPIKey`](#newapikey).

### POST /api/admin/keys/{key_id}/delete

Delete anyone's key

|  |  |
|---|---|
| Who may call | browser session only, admins |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `key_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### POST /api/admin/keys/{key_id}/expiry

Change when anyone's key expires

|  |  |
|---|---|
| Who may call | browser session only, admins |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `key_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `expires_in_days` | integer | yes | 0 to 3650 | Counted from now. 0 means it never expires. |

**Reply 200**: Success

An empty object.

### POST /api/admin/keys/{key_id}/rename

Rename anyone's key

|  |  |
|---|---|
| Who may call | browser session only, admins |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `key_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `expect` | string | no |  | The name you think it has. Answers 409 if it has another. |
| `name` | string | yes |  | 1-80 characters. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `name` | string | yes |  |  |
| `ok` | boolean | yes |  |  |

### POST /api/admin/keys/{key_id}/restore

Undo deleting anyone's key

|  |  |
|---|---|
| Who may call | browser session only, admins |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `key_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

[`APIKey`](#apikey).

### POST /api/admin/keys/{key_id}/revoke

Revoke anyone's key

|  |  |
|---|---|
| Who may call | browser session only, admins |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `key_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### GET /api/keys

Your API keys

|  |  |
|---|---|
| Who may call | browser session only, anyone signed in |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `limit` | query | integer | no | 1 to 1000 | Rows to return. Default 200. |
| `offset` | query | integer | no | at least 0 |  |
| `q` | query | string | no |  | Keep keys whose name contains this, in any case. |
| `live` | query | string | no | one of `true` | Leave out revoked and expired keys. |
| `sort` | query | string | no | one of `name`, `role`, `last_used_at`, `expires_at`, `created_at` | Field to sort the whole list by. Rows without it go last. |
| `dir` | query | string | no | one of `asc`, `desc` | Default asc. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `keys` | array of [`APIKey`](#apikey) | yes |  |  |
| `next` | integer or null | yes |  | Offset of the next page, or null on the last one. |
| `total` | integer | yes |  |  |

### POST /api/keys

Create an API key

|  |  |
|---|---|
| Who may call | browser session only, anyone signed in |
| Errors | 400, 401, 403, 404, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `expires_in_days` | integer or null | no | 0 to 3650 | Absent or null: the configured default. 0: never expires. |
| `name` | string | yes | 1 to 80 characters |  |
| `role` | string | no | one of `admin`, `user`; default `user` |  |

**Reply 201**: Success

[`NewAPIKey`](#newapikey).

### POST /api/keys/{key_id}/delete

Delete one of your keys

|  |  |
|---|---|
| Who may call | browser session only, anyone signed in |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `key_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### POST /api/keys/{key_id}/expiry

Change when one of your keys expires

|  |  |
|---|---|
| Who may call | browser session only, anyone signed in |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `key_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `expires_in_days` | integer | yes | 0 to 3650 | Counted from now. 0 means it never expires. |

**Reply 200**: Success

An empty object.

### POST /api/keys/{key_id}/rename

Rename one of your keys

|  |  |
|---|---|
| Who may call | browser session only, anyone signed in |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `key_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `expect` | string | no |  | The name you think it has. Answers 409 if it has another. |
| `name` | string | yes |  | 1-80 characters. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `name` | string | yes |  |  |
| `ok` | boolean | yes |  |  |

### POST /api/keys/{key_id}/restore

Undo deleting one of your keys

|  |  |
|---|---|
| Who may call | browser session only, anyone signed in |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `key_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

[`APIKey`](#apikey).

### POST /api/keys/{key_id}/revoke

Revoke one of your keys

|  |  |
|---|---|
| Who may call | browser session only, anyone signed in |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `key_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

## Files

| Method | Path | Summary | Who may call |
|---|---|---|---|
| GET | [`/api/account/storage`](#get-apiaccountstorage) | How much of your quota is used | browser session or API key, anyone signed in |
| GET | [`/api/admin/files`](#get-apiadminfiles) | Every account's files | browser session or API key, admins |
| POST | [`/api/admin/files/{file_id}/delete`](#post-apiadminfilesfile_iddelete) | Delete anyone's file | browser session only, admins |
| GET | [`/api/admin/files/{file_id}/download`](#get-apiadminfilesfile_iddownload) | Download anyone's file | browser session only, admins |
| POST | [`/api/admin/files/{file_id}/rename`](#post-apiadminfilesfile_idrename) | Rename any file | browser session only, admins |
| POST | [`/api/admin/files/{file_id}/restore`](#post-apiadminfilesfile_idrestore) | Put back a deleted file | browser session only, admins |
| GET | [`/api/admin/storage`](#get-apiadminstorage) | Every account's files together, and room on the disk | browser session or API key, admins |
| GET | [`/api/files`](#get-apifiles) | Your files, newest first | browser session or API key, anyone signed in |
| POST | [`/api/files`](#post-apifiles) | Start an upload | browser session or API key, anyone signed in |
| GET | [`/api/files/{file_id}`](#get-apifilesfile_id) | One of your files | browser session or API key, anyone signed in |
| POST | [`/api/files/{file_id}/chunks/{index}`](#post-apifilesfile_idchunksindex) | Send the next piece of an upload | browser session or API key, anyone signed in |
| POST | [`/api/files/{file_id}/delete`](#post-apifilesfile_iddelete) | Delete one of your files | browser session or API key, anyone signed in |
| GET | [`/api/files/{file_id}/download`](#get-apifilesfile_iddownload) | Download one of your files | browser session or API key, anyone signed in |
| POST | [`/api/files/{file_id}/finish`](#post-apifilesfile_idfinish) | Finish an upload | browser session or API key, anyone signed in |
| POST | [`/api/files/{file_id}/rename`](#post-apifilesfile_idrename) | Rename one of your files | browser session or API key, anyone signed in |
| POST | [`/api/files/{file_id}/restore`](#post-apifilesfile_idrestore) | Put back a file you deleted | browser session or API key, anyone signed in |

### GET /api/account/storage

How much of your quota is used

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`my_storage`](#my_storage) |
| Errors | 401, 404, 429, 503 |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `quota_bytes` | integer | yes |  |  |
| `used_bytes` | integer | yes |  |  |

### GET /api/admin/files

Every account's files

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_list_files`](#admin_list_files) |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `limit` | query | integer | no | 1 to 1000 | Rows to return. Default 200. |
| `offset` | query | integer | no | at least 0 |  |
| `q` | query | string | no |  | Keep files whose name or owner contains this, in any case. |
| `sort` | query | string | no | one of `name`, `user_name`, `size`, `created_at` | Field to sort the whole list by. Rows without it go last. |
| `dir` | query | string | no | one of `asc`, `desc` | Default asc. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `files` | array of [`File`](#file) + object | yes |  |  |
| `files[].user_id` | integer | yes |  |  |
| `files[].user_name` | string | yes |  |  |
| `next` | integer or null | yes |  | Offset of the next page, or null on the last one. |
| `total` | integer | yes |  |  |

### POST /api/admin/files/{file_id}/delete

Delete anyone's file

|  |  |
|---|---|
| Who may call | browser session only, admins |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `file_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### GET /api/admin/files/{file_id}/download

Download anyone's file

Written to the activity log when the file is someone else's.

|  |  |
|---|---|
| Who may call | browser session only, admins |
| Errors | 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `file_id` | path | string | yes |  |  |

**Reply 200**: The file, always as an attachment

Raw bytes, as `application/octet-stream`.

### POST /api/admin/files/{file_id}/rename

Rename any file

|  |  |
|---|---|
| Who may call | browser session only, admins |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `file_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `expect` | string | no |  | The name you think it has. Answers 409 if it has another. |
| `name` | string | yes |  | Cleaned like an upload's name. 1-255 bytes. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `name` | string | yes |  | As stored. |
| `ok` | boolean | yes |  |  |

### POST /api/admin/files/{file_id}/restore

Put back a deleted file

|  |  |
|---|---|
| Who may call | browser session only, admins |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `file_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

[`File`](#file).

### GET /api/admin/storage

Every account's files together, and room on the disk

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_storage`](#admin_storage) |
| Errors | 401, 403, 404, 429, 503 |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `disk_free_bytes` | integer | no |  | Left out where the system will not say. |
| `disk_total_bytes` | integer | no |  | Left out where the system will not say. |
| `used_bytes` | integer | yes |  |  |

### GET /api/files

Your files, newest first

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`files_list`](#files_list) |
| Errors | 400, 401, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `limit` | query | integer | no | 1 to 1000 | Rows to return. Default 200. |
| `offset` | query | integer | no | at least 0 |  |
| `q` | query | string | no |  | Keep files whose name contains this, in any case. |
| `sort` | query | string | no | one of `name`, `size`, `created_at` | Field to sort the whole list by. Rows without it go last. |
| `dir` | query | string | no | one of `asc`, `desc` | Default asc. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `files` | array of [`File`](#file) | yes |  |  |
| `next` | integer or null | yes |  | Offset of the next page, or null on the last one. |
| `total` | integer | yes |  |  |

### POST /api/files

Start an upload

Then send each piece in order and finish. A file over files.max_file_bytes, or one that would pass files.quota_bytes_per_user, is a 409.

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`files_start_upload`](#files_start_upload) |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `name` | string | yes |  |  |
| `size` | integer | yes | at least 0 |  |
| `type` | string | no |  | Shown in the list. Downloads are always application/octet-stream. |

**Reply 201**: Created

Every field of [`File`](#file), plus these:

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `chunk_bytes` | integer | yes |  | Every piece but the last is exactly this long. |

### GET /api/files/{file_id}

One of your files

received is where to resume an upload.

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`files_get`](#files_get) |
| Errors | 401, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `file_id` | path | string | yes |  |  |

**Reply 200**: Success

Every field of [`File`](#file), plus these:

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `chunk_bytes` | integer | yes |  | Every piece but the last is exactly this long. |

### POST /api/files/{file_id}/chunks/{index}

Send the next piece of an upload

The body is the raw bytes. Pieces go in order, from 0. A browser session sends its csrf token in the X-CSRF-Token header. A 409 carries received, the piece to send next. Counts against server.rate_limit_upload_max.

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | None, HTTP only: File bytes: MCP messages are JSON, and clients cap their size. files_start_upload says where to send each piece, with the same key. |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `file_id` | path | string | yes |  |  |
| `index` | path | integer | yes | at least 0 |  |
| `X-CSRF-Token` | header | string | no |  | From GET /api/me. Browser sessions only. |

**Request body**

Raw bytes, as `application/octet-stream`.

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `received` | integer | yes |  |  |

### POST /api/files/{file_id}/delete

Delete one of your files

It can be restored for ten minutes.

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`files_delete`](#files_delete) |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `file_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### GET /api/files/{file_id}/download

Download one of your files

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`files_download_url`](#files_download_url) |
| Errors | 401, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `file_id` | path | string | yes |  |  |

**Reply 200**: The file, always as an attachment

Raw bytes, as `application/octet-stream`.

### POST /api/files/{file_id}/finish

Finish an upload

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`files_finish_upload`](#files_finish_upload) |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `file_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

[`File`](#file).

### POST /api/files/{file_id}/rename

Rename one of your files

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`files_rename`](#files_rename) |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `file_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `expect` | string | no |  | The name you think it has. Answers 409 if it has another. |
| `name` | string | yes |  | Cleaned like an upload's name. 1-255 bytes. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `name` | string | yes |  | As stored. |
| `ok` | boolean | yes |  |  |

### POST /api/files/{file_id}/restore

Put back a file you deleted

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | [`files_restore`](#files_restore) |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `file_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

[`File`](#file).

## Admin

| Method | Path | Summary | Who may call |
|---|---|---|---|
| GET | [`/api/admin/theme-defaults`](#get-apiadmintheme-defaults) | The themes everyone without their own list sees | browser session or API key, admins |
| POST | [`/api/admin/theme-defaults`](#post-apiadmintheme-defaults) | Replace the default themes | browser session or API key, admins |
| GET | [`/api/admin/users`](#get-apiadminusers) | Every account | browser session or API key, admins |
| POST | [`/api/admin/users`](#post-apiadminusers) | Create an account and its login link | browser session or API key, admins |
| POST | [`/api/admin/users/{user_id}/cancel-login-link`](#post-apiadminusersuser_idcancel-login-link) | Stop an account's login link working | browser session or API key, admins |
| POST | [`/api/admin/users/{user_id}/delete`](#post-apiadminusersuser_iddelete) | Delete an account | browser session or API key, admins |
| POST | [`/api/admin/users/{user_id}/disabled`](#post-apiadminusersuser_iddisabled) | Disable or enable an account | browser session or API key, admins |
| POST | [`/api/admin/users/{user_id}/reissue-login`](#post-apiadminusersuser_idreissue-login) | A new login link for an account | browser session or API key, admins |
| POST | [`/api/admin/users/{user_id}/rename`](#post-apiadminusersuser_idrename) | Rename someone else | browser session or API key, admins |
| POST | [`/api/admin/users/{user_id}/reset-two-factor`](#post-apiadminusersuser_idreset-two-factor) | Turn off an account's two-factor | browser session or API key, admins |
| POST | [`/api/admin/users/{user_id}/revoke-access`](#post-apiadminusersuser_idrevoke-access) | End every credential an account holds | browser session or API key, admins |
| POST | [`/api/admin/users/{user_id}/role`](#post-apiadminusersuser_idrole) | Change an account's role | browser session or API key, admins |
| GET | [`/api/admin/users/{user_id}/sessions`](#get-apiadminusersuser_idsessions) | An account's signed-in devices | browser session or API key, admins |
| POST | [`/api/admin/users/{user_id}/sessions/{session_id}/revoke`](#post-apiadminusersuser_idsessionssession_idrevoke) | Sign out one of an account's devices | browser session or API key, admins |

### GET /api/admin/theme-defaults

The themes everyone without their own list sees

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_get_theme_defaults`](#admin_get_theme_defaults) |
| Errors | 401, 403, 404, 429, 503 |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `themes` | array of [`Theme`](#theme) or null | yes |  | null while the built-in themes apply. |

### POST /api/admin/theme-defaults

Replace the default themes

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_set_theme_defaults`](#admin_set_theme_defaults) |
| Errors | 400, 401, 403, 404, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `themes` | array of [`Theme`](#theme) or null | no |  | 1 to 40 themes. null goes back to the built-in ones. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |
| `themes` | array of [`Theme`](#theme) or null | yes |  |  |

### GET /api/admin/users

Every account

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_list_users`](#admin_list_users) |
| Errors | 400, 401, 403, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `limit` | query | integer | no | 1 to 1000 | Rows to return. Default 200. |
| `offset` | query | integer | no | at least 0 |  |
| `q` | query | string | no |  | Keep accounts whose name or username contains this, in any case. |
| `sort` | query | string | no | one of `display_name`, `role`, `disabled`, `created_at` | Field to sort the whole list by. Rows without it go last. |
| `dir` | query | string | no | one of `asc`, `desc` | Default asc. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `next` | integer or null | yes |  | Offset of the next page, or null on the last one. |
| `total` | integer | yes |  |  |
| `users` | array of [`User`](#user) | yes |  |  |

### POST /api/admin/users

Create an account and its login link

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_create_user`](#admin_create_user) |
| Errors | 400, 401, 403, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `display_name` | string | yes |  |  |
| `expires_in_minutes` | integer | no | 0 to 525600 | 0 never expires. Default auth.login_token_ttl_seconds for a link that works once, auth.multi_use_login_token_ttl_seconds otherwise. |
| `max_uses` | integer | no | 0 to 1000 | Sign-ins the link allows: 1-1000, or 0 for unlimited. Default 1. |
| `role` | string | no | one of `admin`, `user` |  |

**Reply 201**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `expires_at` | string (date-time) or null | yes |  | null when it never expires. |
| `expires_in_seconds` | integer | yes |  | 0 when it never expires. |
| `login_path` | string | yes |  | Path of the login link. A credential: treat it like a password. |
| `max_uses` | integer | yes |  | 0 when unlimited. |
| `unlimited` | boolean | yes |  |  |
| `user_id` | integer | yes |  |  |

### POST /api/admin/users/{user_id}/cancel-login-link

Stop an account's login link working

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_cancel_login_link`](#admin_cancel_login_link) |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `user_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |
| `removed` | integer | yes |  | Links removed: 0 or 1. |

### POST /api/admin/users/{user_id}/delete

Delete an account

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_delete_user`](#admin_delete_user) |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `user_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |
| `removed` | [`Revoked`](#revoked) | yes |  |  |

### POST /api/admin/users/{user_id}/disabled

Disable or enable an account

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_set_user_disabled`](#admin_set_user_disabled) |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `user_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `disabled` | boolean | yes |  |  |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### POST /api/admin/users/{user_id}/reissue-login

A new login link for an account

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_reissue_login`](#admin_reissue_login) |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `user_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `expires_in_minutes` | integer | no | 0 to 525600 | 0 never expires. Default auth.login_token_ttl_seconds for a link that works once, auth.multi_use_login_token_ttl_seconds otherwise. |
| `max_uses` | integer | no | 0 to 1000 | Sign-ins the link allows: 1-1000, or 0 for unlimited. Default 1. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `expires_at` | string (date-time) or null | yes |  | null when it never expires. |
| `expires_in_seconds` | integer | yes |  | 0 when it never expires. |
| `login_path` | string | yes |  | Path of the login link. A credential: treat it like a password. |
| `max_uses` | integer | yes |  | 0 when unlimited. |
| `unlimited` | boolean | yes |  |  |

### POST /api/admin/users/{user_id}/rename

Rename someone else

Only the owner can rename an admin.

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_rename_user`](#admin_rename_user) |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `user_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `display_name` | string | yes |  | 1-80 characters. |
| `expect` | string | no |  | The name you think they have. Answers 409 if it is another. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `display_name` | string | yes |  |  |
| `ok` | boolean | yes |  |  |

### POST /api/admin/users/{user_id}/reset-two-factor

Turn off an account's two-factor

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_reset_two_factor`](#admin_reset_two_factor) |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `user_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### POST /api/admin/users/{user_id}/revoke-access

End every credential an account holds

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_revoke_access`](#admin_revoke_access) |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `user_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |
| `revoked` | [`Revoked`](#revoked) | yes |  |  |

### POST /api/admin/users/{user_id}/role

Change an account's role

Only the owner can take an admin's rights away. Becoming an admin ends the account's sessions and live login link.

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_set_user_role`](#admin_set_user_role) |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `user_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `role` | string | yes | one of `admin`, `user` |  |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |
| `role` | string | yes |  |  |

### GET /api/admin/users/{user_id}/sessions

An account's signed-in devices

The owner's are the owner's alone, and only the owner sees addresses.

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_list_user_sessions`](#admin_list_user_sessions) |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `user_id` | path | integer | yes |  |  |
| `limit` | query | integer | no | 1 to 1000 | Rows to return. Default 200. |
| `offset` | query | integer | no | at least 0 |  |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `next` | integer or null | yes |  | Offset of the next page, or null on the last one. |
| `sessions` | array of [`Session`](#session) | yes |  |  |
| `total` | integer | yes |  |  |

### POST /api/admin/users/{user_id}/sessions/{session_id}/revoke

Sign out one of an account's devices

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_revoke_user_session`](#admin_revoke_user_session) |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `user_id` | path | integer | yes |  |  |
| `session_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

## Invite links

| Method | Path | Summary | Who may call |
|---|---|---|---|
| GET | [`/api/admin/invite-links`](#get-apiadmininvite-links) | Every invite link | browser session or API key, admins |
| POST | [`/api/admin/invite-links`](#post-apiadmininvite-links) | Create an invite link | browser session or API key, admins |
| POST | [`/api/admin/invite-links/{link_id}/delete`](#post-apiadmininvite-linkslink_iddelete) | Delete an invite link | browser session or API key, admins |
| POST | [`/api/admin/invite-links/{link_id}/limits`](#post-apiadmininvite-linkslink_idlimits) | Change a link's places and expiry | browser session or API key, admins |
| POST | [`/api/admin/invite-links/{link_id}/rename`](#post-apiadmininvite-linkslink_idrename) | Rename a link | browser session or API key, admins |
| POST | [`/api/admin/invite-links/{link_id}/restore`](#post-apiadmininvite-linkslink_idrestore) | Undo deleting an invite link | browser session or API key, admins |
| POST | [`/api/admin/invite-links/{link_id}/revoke`](#post-apiadmininvite-linkslink_idrevoke) | Switch an invite link off | browser session or API key, admins |
| POST | [`/api/admin/invite-links/{link_id}/switch-on`](#post-apiadmininvite-linkslink_idswitch-on) | Switch a link back on | browser session or API key, admins |
| POST | [`/api/join/claim`](#post-apijoinclaim) | Sign in as an existing account | anyone |
| POST | [`/api/join/create`](#post-apijoincreate) | Join as a new account | anyone |
| POST | [`/api/join/info`](#post-apijoininfo) | What an invite link is | anyone |
| POST | [`/api/join/match`](#post-apijoinmatch) | Accounts matching a name | anyone |

### GET /api/admin/invite-links

Every invite link

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_list_invite_links`](#admin_list_invite_links) |
| Errors | 400, 401, 403, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `limit` | query | integer | no | 1 to 1000 | Rows to return. Default 200. |
| `offset` | query | integer | no | at least 0 |  |
| `q` | query | string | no |  | Keep links whose label or maker contains this, in any case. |
| `sort` | query | string | no | one of `label`, `used`, `expires_at` | Field to sort the whole list by. Rows without it go last. |
| `dir` | query | string | no | one of `asc`, `desc` | Default asc. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `invite_links` | array of [`InviteLink`](#invitelink) | yes |  |  |
| `next` | integer or null | yes |  | Offset of the next page, or null on the last one. |
| `total` | integer | yes |  |  |

### POST /api/admin/invite-links

Create an invite link

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_create_invite_link`](#admin_create_invite_link) |
| Errors | 400, 401, 403, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `days` | integer | yes |  |  |
| `label` | string | yes |  |  |
| `max_uses` | integer | yes |  |  |

**Reply 201**: Success

Every field of [`InviteLink`](#invitelink) and [`JoinPath`](#joinpath).

### POST /api/admin/invite-links/{link_id}/delete

Delete an invite link

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_delete_invite_link`](#admin_delete_invite_link) |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `link_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### POST /api/admin/invite-links/{link_id}/limits

Change a link's places and expiry

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_set_invite_link_limits`](#admin_set_invite_link_limits) |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `link_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `days` | integer | no | 1 to 365 | Counted from now. Leave out to keep the expiry. |
| `max_uses` | integer | yes | 1 to 1000 | Not fewer than have joined. |

**Reply 200**: Success

An empty object.

### POST /api/admin/invite-links/{link_id}/rename

Rename a link

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_rename_invite_link`](#admin_rename_invite_link) |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `link_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `expect` | string | no |  | The label you think it has. Answers 409 if it has another. |
| `label` | string | yes |  | 1-80 characters. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `label` | string | yes |  |  |
| `ok` | boolean | yes |  |  |

### POST /api/admin/invite-links/{link_id}/restore

Undo deleting an invite link

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_restore_invite_link`](#admin_restore_invite_link) |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `link_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

[`InviteLink`](#invitelink).

### POST /api/admin/invite-links/{link_id}/revoke

Switch an invite link off

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_revoke_invite_link`](#admin_revoke_invite_link) |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `link_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### POST /api/admin/invite-links/{link_id}/switch-on

Switch a link back on

|  |  |
|---|---|
| Who may call | browser session or API key, admins |
| MCP tool | [`admin_switch_on_invite_link`](#admin_switch_on_invite_link) |
| Errors | 400, 401, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `link_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

An empty object.

### POST /api/join/claim

Sign in as an existing account

|  |  |
|---|---|
| Who may call | anyone |
| Errors | 400, 403, 404, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `credential` | string | yes |  |  |
| `display_name` | string | no |  |  |
| `pin` | string | no |  |  |
| `user_id` | integer | no |  |  |

**Reply 200**: Success

[`Joined`](#joined).

### POST /api/join/create

Join as a new account

|  |  |
|---|---|
| Who may call | anyone |
| Errors | 400, 404, 409, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `credential` | string | yes |  |  |
| `display_name` | string | no |  |  |
| `pin` | string | no |  |  |
| `user_id` | integer | no |  |  |

**Reply 201**: Success

[`Joined`](#joined).

### POST /api/join/info

What an invite link is

|  |  |
|---|---|
| Who may call | anyone |
| Errors | 404, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `credential` | string | yes |  |  |
| `display_name` | string | no |  |  |
| `pin` | string | no |  |  |
| `user_id` | integer | no |  |  |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `label` | string | yes |  |  |
| `remaining` | integer | yes |  |  |

### POST /api/join/match

Accounts matching a name

|  |  |
|---|---|
| Who may call | anyone |
| Errors | 400, 404, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `credential` | string | yes |  |  |
| `display_name` | string | no |  |  |
| `pin` | string | no |  |  |
| `user_id` | integer | no |  |  |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `can_create` | boolean | yes |  |  |
| `matches` | array of [`Member`](#member) | yes |  |  |

## Owner

| Method | Path | Summary | Who may call |
|---|---|---|---|
| GET | [`/api/owner/jobs`](#get-apiownerjobs) | Background jobs | browser session only, the owner |
| POST | [`/api/owner/jobs/{name}/run`](#post-apiownerjobsnamerun) | Run a job now | browser session only, the owner |
| GET | [`/api/owner/logs/days`](#get-apiownerlogsdays) | The days on record | browser session only, the owner |
| GET | [`/api/owner/logs/days/{date}`](#get-apiownerlogsdaysdate) | One day's entries | browser session only, the owner |
| POST | [`/api/owner/logs/days/{date}/delete`](#post-apiownerlogsdaysdatedelete) | Delete one day | browser session only, the owner |
| GET | [`/api/owner/logs/days/{date}/download`](#get-apiownerlogsdaysdatedownload) | One day's entries as a file | browser session only, the owner |
| GET | [`/api/owner/logs/ip-names`](#get-apiownerlogsip-names) | What the owner calls addresses | browser session only, the owner |
| POST | [`/api/owner/logs/ip-names`](#post-apiownerlogsip-names) | Name an address or a range, or rename one | browser session only, the owner |
| POST | [`/api/owner/logs/ip-names/delete`](#post-apiownerlogsip-namesdelete) | Take a name off an address or a range | browser session only, the owner |
| POST | [`/api/owner/logs/prune`](#post-apiownerlogsprune) | Apply the retention rules now | browser session only, the owner |
| GET | [`/api/owner/logs/stream`](#get-apiownerlogsstream) | New log entries as they happen | browser session only, the owner |
| GET | [`/api/owner/settings`](#get-apiownersettings) | config.yaml, as a form | browser session only, the owner |
| POST | [`/api/owner/settings`](#post-apiownersettings) | Save settings | browser session only, the owner |
| POST | [`/api/owner/users/{user_id}/password-access`](#post-apiownerusersuser_idpassword-access) | Allow or block an account's password | browser session only, the owner |

### GET /api/owner/jobs

Background jobs

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 404, 429, 503 |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `jobs` | array of [`Job`](#job) | yes |  |  |

### POST /api/owner/jobs/{name}/run

Run a job now

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `name` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 202**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### GET /api/owner/logs/days

The days on record

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `q` | query | string | no |  | The Logs screen's search. |

**Reply 200**: Success

[`LogDays`](#logdays).

### GET /api/owner/logs/days/{date}

One day's entries

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `date` | path | string | yes |  |  |
| `q` | query | string | no |  | The Logs screen's search. |
| `before` | query | integer | no |  |  |
| `limit` | query | integer | no |  |  |

**Reply 200**: Success

[`LogDay`](#logday).

### POST /api/owner/logs/days/{date}/delete

Delete one day

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `date` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `deleted` | boolean | yes |  |  |

### GET /api/owner/logs/days/{date}/download

One day's entries as a file

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `date` | path | string | yes |  |  |

**Reply 200**: Success

Text, as `application/x-ndjson`.

### GET /api/owner/logs/ip-names

What the owner calls addresses

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 403, 404, 429, 503 |

**Reply 200**: Success

[`IPNames`](#ipnames).

### POST /api/owner/logs/ip-names

Name an address or a range, or rename one

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 403, 404, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `name` | string | yes | at most 80 characters |  |
| `target` | string | yes | e.g. `10.0.0.0/24` | An address, or a range such as 10.0.0.0/24. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `name` | string | no |  |  |
| `target` | string | no |  |  |

### POST /api/owner/logs/ip-names/delete

Take a name off an address or a range

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 403, 404, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `target` | string | yes | e.g. `10.0.0.0/24` | An address, or a range such as 10.0.0.0/24. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `deleted` | boolean | no |  |  |

### POST /api/owner/logs/prune

Apply the retention rules now

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 403, 404, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

[`PruneResult`](#pruneresult).

### GET /api/owner/logs/stream

New log entries as they happen

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `q` | query | string | no |  | The Logs screen's search. |

**Reply 200**: Success

Text, as `text/event-stream`.

### GET /api/owner/settings

config.yaml, as a form

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 404, 429, 503 |

**Reply 200**: Success

[`SettingsSnapshot`](#settingssnapshot).

### POST /api/owner/settings

Save settings

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 403, 404, 409, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `changes` | object | yes |  | Setting paths and their new values. |
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `version` | string | yes |  |  |

**Reply 200**: Success

[`SettingsResult`](#settingsresult).

### POST /api/owner/users/{user_id}/password-access

Allow or block an account's password

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `user_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `allowed` | boolean | yes |  |  |
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |
| `password_blocked` | boolean | yes |  |  |

## Webhooks

| Method | Path | Summary | Who may call |
|---|---|---|---|
| GET | [`/api/owner/webhooks`](#get-apiownerwebhooks) | Webhooks | browser session only, the owner |
| POST | [`/api/owner/webhooks`](#post-apiownerwebhooks) | Create a webhook | browser session only, the owner |
| GET | [`/api/owner/webhooks/events`](#get-apiownerwebhooksevents) | Events a webhook can ask for | browser session only, the owner |
| POST | [`/api/owner/webhooks/{webhook_id}/delete`](#post-apiownerwebhookswebhook_iddelete) | Delete a webhook | browser session only, the owner |
| GET | [`/api/owner/webhooks/{webhook_id}/deliveries`](#get-apiownerwebhookswebhook_iddeliveries) | Recent deliveries | browser session only, the owner |
| POST | [`/api/owner/webhooks/{webhook_id}/deliveries/{delivery_id}/resend`](#post-apiownerwebhookswebhook_iddeliveriesdelivery_idresend) | Send a failed delivery again | browser session only, the owner |
| POST | [`/api/owner/webhooks/{webhook_id}/enabled`](#post-apiownerwebhookswebhook_idenabled) | Switch a webhook on or off | browser session only, the owner |
| POST | [`/api/owner/webhooks/{webhook_id}/rename`](#post-apiownerwebhookswebhook_idrename) | Rename a webhook | browser session only, the owner |
| POST | [`/api/owner/webhooks/{webhook_id}/restore`](#post-apiownerwebhookswebhook_idrestore) | Undo deleting a webhook | browser session only, the owner |
| POST | [`/api/owner/webhooks/{webhook_id}/rotate-secret`](#post-apiownerwebhookswebhook_idrotate-secret) | Replace a webhook's signing secret | browser session only, the owner |
| POST | [`/api/owner/webhooks/{webhook_id}/target`](#post-apiownerwebhookswebhook_idtarget) | Change a webhook's address and events | browser session only, the owner |
| POST | [`/api/owner/webhooks/{webhook_id}/test`](#post-apiownerwebhookswebhook_idtest) | Send a test event | browser session only, the owner |

### GET /api/owner/webhooks

Webhooks

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `limit` | query | integer | no | 1 to 1000 | Rows to return. Default 200. |
| `offset` | query | integer | no | at least 0 |  |
| `q` | query | string | no |  | Keep webhooks whose name or address contains this, in any case. |
| `sort` | query | string | no | one of `name`, `url`, `enabled` | Field to sort the whole list by. Rows without it go last. |
| `dir` | query | string | no | one of `asc`, `desc` | Default asc. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `next` | integer or null | yes |  | Offset of the next page, or null on the last one. |
| `total` | integer | yes |  |  |
| `webhooks` | array of [`Webhook`](#webhook) | yes |  |  |

### POST /api/owner/webhooks

Create a webhook

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 403, 404, 429, 503 |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `events` | array of string | yes |  |  |
| `name` | string | yes |  |  |
| `url` | string | yes |  |  |

**Reply 201**: Success

Every field of [`Webhook`](#webhook), plus these:

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `secret` | string | yes |  | Shown in this response only. |

### GET /api/owner/webhooks/events

Events a webhook can ask for

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 404, 429, 503 |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `events` | array of string | yes |  |  |

### POST /api/owner/webhooks/{webhook_id}/delete

Delete a webhook

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `webhook_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### GET /api/owner/webhooks/{webhook_id}/deliveries

Recent deliveries

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `webhook_id` | path | string | yes |  |  |
| `limit` | query | integer | no | 1 to 1000 | Rows to return. Default 200. |
| `offset` | query | integer | no | at least 0 |  |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `deliveries` | array of [`WebhookDelivery`](#webhookdelivery) | yes |  |  |
| `next` | integer or null | yes |  | Offset of the next page, or null on the last one. |
| `total` | integer | yes |  |  |

### POST /api/owner/webhooks/{webhook_id}/deliveries/{delivery_id}/resend

Send a failed delivery again

A new delivery with the same body. The failed one stays listed.

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 401, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `webhook_id` | path | string | yes |  |  |
| `delivery_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 202**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### POST /api/owner/webhooks/{webhook_id}/enabled

Switch a webhook on or off

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `webhook_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `enabled` | boolean | yes |  |  |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### POST /api/owner/webhooks/{webhook_id}/rename

Rename a webhook

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `webhook_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `expect` | string | no |  | The name you think it has. Answers 409 if it has another. |
| `name` | string | yes |  | 1-80 characters. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `name` | string | yes |  |  |
| `ok` | boolean | yes |  |  |

### POST /api/owner/webhooks/{webhook_id}/restore

Undo deleting a webhook

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `webhook_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

[`Webhook`](#webhook).

### POST /api/owner/webhooks/{webhook_id}/rotate-secret

Replace a webhook's signing secret

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `webhook_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `secret` | string | yes |  | Shown only here. The old one has stopped working. |

### POST /api/owner/webhooks/{webhook_id}/target

Change a webhook's address and events

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 403, 404, 409, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `webhook_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |
| `events` | array of string | yes |  |  |
| `expect` | object | no |  | What you think it has now. Answers 409 if it has something else. |
| `expect.events` | array of string | yes |  |  |
| `expect.url` | string | yes |  |  |
| `url` | string | yes |  | Checked like a new webhook's. |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### POST /api/owner/webhooks/{webhook_id}/test

Send a test event

|  |  |
|---|---|
| Who may call | browser session only, the owner |
| Errors | 400, 403, 404, 429, 503 |

**Parameters**

| Name | In | Type | Required | Limits | Description |
|---|---|---|---|---|---|
| `webhook_id` | path | string | yes |  |  |

**Request body**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `csrf_token` | string | no |  | From GET /api/me. Browser sessions only. |

**Reply 202**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

## Monitoring

| Method | Path | Summary | Who may call |
|---|---|---|---|
| GET | [`/api/health`](#get-apihealth) | Is the process up | anyone |
| GET | [`/api/mcp.json`](#get-apimcpjson) | The MCP tools: every tool /mcp has, its arguments, and the route it stands for | browser session or API key, anyone signed in |
| GET | [`/api/openapi.json`](#get-apiopenapijson) | This document | browser session or API key, anyone signed in |
| GET | [`/api/ready`](#get-apiready) | Can the server do real work | anyone |
| GET | [`/mcp`](#get-mcp) | Refused: this server has no stream | anyone |
| POST | [`/mcp`](#post-mcp) | Model Context Protocol, at the key's role | API key only, anyone signed in |
| DELETE | [`/mcp`](#delete-mcp) | Refused: this server keeps no sessions | anyone |
| GET | [`/metrics`](#get-metrics) | Prometheus metrics | API key only, admins |

### GET /api/health

Is the process up

|  |  |
|---|---|
| Who may call | anyone |
| Errors | 429, 503 |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### GET /api/mcp.json

The MCP tools: every tool /mcp has, its arguments, and the route it stands for

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | None, HTTP only: The MCP reference itself; tools/list gives the same list. |
| Errors | 401, 404, 429, 503 |

**Reply 200**: Success

An empty object.

### GET /api/openapi.json

This document

|  |  |
|---|---|
| Who may call | browser session or API key, anyone signed in |
| MCP tool | None, HTTP only: The API reference itself; GET /api/mcp.json is the MCP one. |
| Errors | 401, 404, 429, 503 |

**Reply 200**: Success

An empty object.

### GET /api/ready

Can the server do real work

|  |  |
|---|---|
| Who may call | anyone |
| Errors | 429, 503 |

**Reply 200**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### GET /mcp

Refused: this server has no stream

|  |  |
|---|---|
| Who may call | anyone |
| Errors | 404, 429, 503 |

**Reply 405**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### POST /mcp

Model Context Protocol, at the key's role

|  |  |
|---|---|
| Who may call | API key only, anyone signed in |
| MCP tool | None, HTTP only: The MCP endpoint itself. |
| Errors | 400, 401, 404, 429, 503 |

**Request body**

An empty object.

**Reply 200**: Success

An empty object.

### DELETE /mcp

Refused: this server keeps no sessions

|  |  |
|---|---|
| Who may call | anyone |
| Errors | 404, 429, 503 |

**Reply 405**: Success

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

### GET /metrics

Prometheus metrics

|  |  |
|---|---|
| Who may call | API key only, admins |
| MCP tool | None, HTTP only: Scraped by Prometheus over HTTP. |
| Errors | 401, 403, 404, 429, 503 |

**Reply 200**: Success

Text, as `text/plain`.

## Schemas

The shapes the routes above refer to by name.

### APIKey

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `created_at` | string (date-time) | yes |  |  |
| `created_by` | integer | no |  | The admin who made this key for its owner. Absent if the owner made it. |
| `created_by_name` | string | no |  |  |
| `display_name` | string | no |  | The key's owner. On the admin list only. |
| `expired` | boolean | yes |  |  |
| `expires_at` | string (date-time) | no |  |  |
| `id` | string | yes |  |  |
| `last_used_at` | string (date-time) | no |  |  |
| `live` | boolean | yes |  |  |
| `name` | string | yes |  |  |
| `revoked` | boolean | yes |  |  |
| `revoked_at` | string (date-time) | no |  |  |
| `role` | string | yes | one of `admin`, `user` |  |
| `user_id` | integer | yes |  |  |

### Error

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `error` | string | yes |  |  |
| `request_id` | string | no |  | On a 5xx: the reference that finds this failure in the log. |

### File

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `chunks` | integer | yes |  |  |
| `created_at` | string (date-time) | yes |  |  |
| `id` | string | yes |  |  |
| `name` | string | yes |  |  |
| `received` | integer | yes |  |  |
| `size` | integer | yes |  |  |
| `state` | string | yes | one of `uploading`, `ready` |  |
| `type` | string | yes |  |  |

### IPNames

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `names` | array of object | no |  |  |
| `names[].created_at` | string (date-time) | no |  |  |
| `names[].name` | string | no |  |  |
| `names[].target` | string | no |  |  |

### InviteLink

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `created_by` | integer | yes |  |  |
| `created_by_name` | string | no |  |  |
| `expires_at` | string (date-time) | no |  |  |
| `id` | string | yes |  |  |
| `label` | string | yes |  |  |
| `live` | boolean | yes |  |  |
| `max_uses` | integer | yes |  |  |
| `remaining` | integer | yes |  |  |
| `revoked` | boolean | yes |  |  |
| `used` | integer | yes |  |  |

### Job

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `interrupted` | boolean | yes |  |  |
| `interval_seconds` | integer | yes |  |  |
| `last_error` | string | no |  |  |
| `last_finished` | string (date-time) | no |  |  |
| `last_note` | string | no |  |  |
| `last_ok` | boolean | no |  |  |
| `last_started` | string (date-time) | no |  |  |
| `name` | string | yes |  |  |
| `next_due` | string (date-time) | yes |  |  |
| `running` | boolean | yes |  |  |
| `runs` | integer | yes |  |  |

### JoinPath

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `join_path` | string | yes |  | The link to hand out, shown in this response only. A credential: treat it like a password. |

### Joined

One of these:

1. An object:

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `display_name` | string | yes |  |  |
| `ok` | boolean | yes |  |  |

2. An object:

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `challenge` | string | yes |  |  |
| `two_factor` | boolean | yes |  |  |

### LogDay

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `date` | string | yes |  |  |
| `entries` | array of object | yes |  |  |
| `next` | integer or null | no |  |  |

### LogDays

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `days` | array of object | yes |  |  |
| `dropped` | integer | no |  |  |
| `settings` | object | no |  |  |
| `total_bytes` | integer | no |  |  |

### LoginLink

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `created_at` | string (date-time) | yes |  |  |
| `expires_at` | string (date-time) or null | yes |  | null when it never expires. |
| `last_used_at` | string (date-time) or null | yes |  |  |
| `max_uses` | integer | yes |  | Sign-ins it allows. 0 when unlimited. |
| `unlimited` | boolean | yes |  |  |
| `uses` | integer | yes |  | Sign-ins so far. |

### Maintenance

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `error` | string | yes |  |  |
| `maintenance` | string | yes | one of `read_only`, `full` |  |

### Me

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `account_role` | string | yes |  |  |
| `announcement` | string | no |  | server.announcement. Empty when there is none. |
| `api_key_id` | string | no |  | API keys only. |
| `api_key_name` | string | no |  | API keys only. |
| `api_keys_enabled` | boolean | no |  |  |
| `auth` | string | no |  |  |
| `authenticated` | boolean | yes |  |  |
| `csrf_token` | string | no |  | Browser sessions only. |
| `description` | string | no |  | app.description. Empty when there is none. |
| `display_name` | string | yes |  |  |
| `files_enabled` | boolean | no |  | files.enabled: whether the Files screen and routes are on. |
| `has_password` | boolean | no |  |  |
| `has_pin` | boolean | no |  |  |
| `id` | integer | yes |  |  |
| `invite_links_enabled` | boolean | no |  | Whether invite links are on. |
| `is_owner` | boolean | yes |  |  |
| `login_link` | [`LoginLink`](#loginlink) or null | no |  | Your own live login link, or null. |
| `logs_enabled` | boolean | no |  |  |
| `logs_ui_clicks` | boolean | no |  |  |
| `maintenance` | string | yes | one of `none`, `read_only`, `full` |  |
| `mcp_enabled` | boolean | no |  | server.mcp: whether POST /mcp answers. |
| `mcp_url` | string | no |  | Where to read the MCP reference: config/mcp.json in app.repo, or /api/mcp.json when there is no repo. Only with server.mcp on. |
| `openapi_url` | string | no |  | Where to read this API's reference: config/openapi.json in app.repo, or /api/openapi.json when there is no repo. |
| `password_blocked` | boolean | no |  |  |
| `passwords_enabled` | boolean | no |  |  |
| `repo` | string | no |  | app.repo without a trailing / or .git: where the source lives. Empty when it is not set. |
| `role` | string | yes |  | The effective role: never higher than an API key's own. |
| `shortcuts` | map of string | no |  | The keyboard shortcuts you changed, by action. Mod is Cmd on a Mac and Ctrl elsewhere. |
| `show_about` | boolean | no |  | app.show_about: whether the menu has About. |
| `theme_id` | string | no |  | The theme you use. Empty: the first. |
| `themes` | array of object or null | no |  | Your themes (see the Theme schema): your own list, else the app's defaults, else null for the built-in ones. |
| `themes_custom` | boolean | no |  | Whether you have your own list rather than following the defaults. |
| `two_factor` | boolean | no |  |  |
| `two_factor_enabled` | boolean | no |  |  |
| `username` | string | no |  |  |
| `version` | string | no |  | The running build. dev when none was stamped. |
| `webhooks_enabled` | boolean | no |  | webhooks.enabled: whether webhooks are sent. |

### Member

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `display_name` | string | yes |  |  |
| `id` | integer | yes |  |  |

### NewAPIKey

Every field of [`APIKey`](#apikey), plus these:

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `credential` | string | yes |  | Shown in this response only, and never recoverable. |

### PruneResult

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `bytes_after` | integer | no |  |  |
| `bytes_before` | integer | no |  |  |
| `compressed` | integer | no |  |  |
| `deleted` | integer | no |  |  |

### Revoked

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `api_keys` | integer | no |  |  |
| `login_links` | integer | no |  |  |
| `passwords` | integer | no |  |  |
| `sessions` | integer | no |  |  |

### Session

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `created_at` | string (date-time) | yes |  |  |
| `current` | boolean | yes |  |  |
| `expires_at` | string (date-time) | yes |  |  |
| `id` | string | yes |  |  |
| `ip` | string | no |  |  |
| `ip_name` | string | no |  | What the owner calls this address on the Logs screen. The owner's own list only. |
| `label` | string | no |  | The name you gave this device. |
| `last_seen_at` | string (date-time) | no |  |  |
| `user_agent` | string | no |  |  |

### SettingsResult

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `applied` | array of string or null | no |  |  |
| `changes` | array of object or null | no |  |  |
| `restart` | array of string or null | no |  |  |
| `scripts` | array of string or null | no |  |  |
| `unchanged` | array of string or null | no |  |  |
| `version` | string | no |  |  |

### SettingsSnapshot

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `error` | object or null | no |  |  |
| `error.at` | string (date-time) | no |  |  |
| `error.message` | string | no |  |  |
| `error.origin` | string | no |  |  |
| `file` | string | no |  |  |
| `in_container` | boolean | no |  |  |
| `secret_file` | string | no |  |  |
| `sections` | array of object | no |  |  |
| `sections[].help` | string | no |  |  |
| `sections[].keys` | array of object | no |  |  |
| `sections[].keys[].apply` | string | no |  |  |
| `sections[].keys[].default` | any | no |  |  |
| `sections[].keys[].help` | string | no |  |  |
| `sections[].keys[].in_file` | boolean | no |  |  |
| `sections[].keys[].key` | string | no |  |  |
| `sections[].keys[].locked` | boolean | no |  |  |
| `sections[].keys[].name` | string | no |  |  |
| `sections[].keys[].pending_restart` | boolean | no |  |  |
| `sections[].keys[].running` | any | no |  |  |
| `sections[].keys[].type` | string | no |  |  |
| `sections[].keys[].value` | any | no |  |  |
| `sections[].name` | string | no |  |  |
| `stale` | boolean | no |  |  |
| `version` | string | no |  | Send it back when saving: a changed file is refused with a 409. |
| `writable` | boolean | no |  |  |

### SignIn

One of these:

1. An object:

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `ok` | boolean | yes |  |  |

2. An object:

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `challenge` | string | yes |  |  |
| `two_factor` | boolean | yes |  |  |

### Theme

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `accent` | string | yes |  | The main color: what you can press. |
| `background` | string | no |  | The page. Left out: the mode's own. |
| `button` | object | no |  | How the theme's buttons look. A choice left out is the default (rounded, normal, raised, normal). |
| `button.case` | string | no | one of `normal`, `upper` |  |
| `button.depth` | string | no | one of `flat`, `raised`, `glossy` |  |
| `button.shape` | string | no | one of `square`, `soft`, `rounded`, `pill` |  |
| `button.weight` | string | no | one of `normal`, `medium`, `bold` |  |
| `dark` | object | no |  | A light theme's colors for dark mode, kept while it is light. Switching the theme to dark swaps them in. A light theme's light (or a dark theme's dark) is dropped. |
| `dark.accent` | string | yes |  | The main color: what you can press. |
| `dark.background` | string | no |  | The page. Left out: the mode's own. |
| `dark.overrides` | map of string | no |  | Worked-out colors replaced by name: text, accent, accentFg, second, secondFg, secondText, link, navLink, background, surface, nested, chrome, info, infoBorder, border, and for each button variant V (Secondary, Success, Danger, Warning, Info, Light, Dark) btnV, btnVFg and btnVText. Leaving one out puts the readable color back. |
| `dark.second` | string | yes |  | Says where you are: active nav item, headings, info. |
| `dark.surface` | string | no |  | Cards. Left out: the mode's own. |
| `id` | string | yes |  | Lower-case letters, digits, - and _, up to 32. |
| `light` | object | no |  | A dark theme's colors for light mode, kept while it is dark. Switching the theme to light swaps them in. |
| `light.accent` | string | yes |  | The main color: what you can press. |
| `light.background` | string | no |  | The page. Left out: the mode's own. |
| `light.overrides` | map of string | no |  | Worked-out colors replaced by name: text, accent, accentFg, second, secondFg, secondText, link, navLink, background, surface, nested, chrome, info, infoBorder, border, and for each button variant V (Secondary, Success, Danger, Warning, Info, Light, Dark) btnV, btnVFg and btnVText. Leaving one out puts the readable color back. |
| `light.second` | string | yes |  | Says where you are: active nav item, headings, info. |
| `light.surface` | string | no |  | Cards. Left out: the mode's own. |
| `mode` | string | yes | one of `light`, `dark` |  |
| `name` | string | yes |  | 1 to 60 characters. |
| `overrides` | map of string | no |  | Worked-out colors replaced by name: text, accent, accentFg, second, secondFg, secondText, link, navLink, background, surface, nested, chrome, info, infoBorder, border, and for each button variant V (Secondary, Success, Danger, Warning, Info, Light, Dark) btnV, btnVFg and btnVText. Leaving one out puts the readable color back. |
| `second` | string | yes |  | Says where you are: active nav item, headings, info. |
| `surface` | string | no |  | Cards. Left out: the mode's own. |

### User

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `created_at` | string (date-time) | no |  |  |
| `disabled` | boolean | yes |  |  |
| `display_name` | string | yes |  |  |
| `has_password` | boolean | no |  |  |
| `id` | integer | yes |  |  |
| `login_link` | [`LoginLink`](#loginlink) or null | no |  | The account's live login link, or null. |
| `owner` | boolean | yes |  |  |
| `password_blocked` | boolean | no |  |  |
| `role` | string | yes | one of `admin`, `user` |  |
| `two_factor` | boolean | no |  |  |
| `username` | string | no |  | Empty until a password is set. |

### Webhook

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `created_at` | string (date-time) | yes |  |  |
| `enabled` | boolean | yes |  |  |
| `events` | array of string | yes |  |  |
| `id` | string | yes |  |  |
| `name` | string | yes |  |  |
| `url` | string | yes |  |  |

### WebhookDelivery

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `attempts` | integer | yes |  |  |
| `created_at` | string (date-time) | yes |  |  |
| `event` | string | yes |  |  |
| `finished_at` | string (date-time) | no |  |  |
| `id` | string | yes |  |  |
| `last_error` | string | no |  |  |
| `last_status` | integer | no |  |  |
| `next_attempt_at` | string (date-time) | no |  |  |
| `status` | string | yes |  |  |

## MCP tools

Every tool POST /mcp offers. Written by scripts/api_docs.sh from the registered tools; do not edit by hand. A key sees the tools its role allows (adminOnly ones need an admin key), and a tool whose feature is switched off answers like one that does not exist. route is the HTTP route the tool stands for, with the same rules; http_only are the routes a key may call that have no tool, and why.

|  |  |
|---|---|
| Endpoint | `POST /mcp` |
| Transport | Streamable HTTP, stateless: one JSON-RPC message per POST, a JSON reply |
| Auth | Authorization: Bearer <API key> |

| Tool | Route | Who | What |
|---|---|---|---|
| [`whoami`](#whoami) | `GET /api/me` | any key | Who am I |
| [`list_team`](#list_team) | `GET /api/team` | any key | List team |
| [`admin_list_users`](#admin_list_users) | `GET /api/admin/users` | admin keys | List users (admin) |
| [`admin_set_user_disabled`](#admin_set_user_disabled) | `POST /api/admin/users/{user_id}/disabled` | admin keys | Disable or enable a user (admin) |
| [`admin_revoke_access`](#admin_revoke_access) | `POST /api/admin/users/{user_id}/revoke-access` | admin keys | Revoke a user's access (admin) |
| [`rename_account`](#rename_account) | `POST /api/account/rename` | any key | Rename your account |
| [`set_shortcuts`](#set_shortcuts) | `POST /api/account/shortcuts` | any key | Change your keyboard shortcuts |
| [`set_themes`](#set_themes) | `POST /api/account/themes` | any key | Replace your own themes |
| [`set_theme`](#set_theme) | `POST /api/account/theme` | any key | Pick your theme |
| [`admin_get_theme_defaults`](#admin_get_theme_defaults) | `GET /api/admin/theme-defaults` | admin keys | Default themes (admin) |
| [`admin_set_theme_defaults`](#admin_set_theme_defaults) | `POST /api/admin/theme-defaults` | admin keys | Replace the default themes (admin) |
| [`create_my_login_link`](#create_my_login_link) | `POST /api/account/login-link` | any key | A login link for yourself |
| [`cancel_my_login_link`](#cancel_my_login_link) | `POST /api/account/login-link/cancel` | any key | Stop your own login link working |
| [`admin_create_user`](#admin_create_user) | `POST /api/admin/users` | admin keys | Create an account (admin) |
| [`admin_reissue_login`](#admin_reissue_login) | `POST /api/admin/users/{user_id}/reissue-login` | admin keys | A new login link for an account (admin) |
| [`admin_cancel_login_link`](#admin_cancel_login_link) | `POST /api/admin/users/{user_id}/cancel-login-link` | admin keys | Stop an account's login link working (admin) |
| [`admin_rename_user`](#admin_rename_user) | `POST /api/admin/users/{user_id}/rename` | admin keys | Rename someone (admin) |
| [`admin_set_user_role`](#admin_set_user_role) | `POST /api/admin/users/{user_id}/role` | admin keys | Change an account's role (admin) |
| [`admin_delete_user`](#admin_delete_user) | `POST /api/admin/users/{user_id}/delete` | admin keys | Delete an account (admin) |
| [`admin_reset_two_factor`](#admin_reset_two_factor) | `POST /api/admin/users/{user_id}/reset-two-factor` | admin keys | Turn off an account's two-factor (admin) |
| [`list_my_sessions`](#list_my_sessions) | `GET /api/account/sessions` | any key | Your signed-in devices |
| [`label_my_session`](#label_my_session) | `POST /api/account/sessions/{session_id}/label` | any key | Name one of your devices |
| [`revoke_my_session`](#revoke_my_session) | `POST /api/account/sessions/{session_id}/revoke` | any key | Sign out one of your devices |
| [`revoke_my_other_sessions`](#revoke_my_other_sessions) | `POST /api/account/sessions/revoke-others` | any key | Sign out all your devices |
| [`admin_list_user_sessions`](#admin_list_user_sessions) | `GET /api/admin/users/{user_id}/sessions` | admin keys | An account's signed-in devices (admin) |
| [`admin_revoke_user_session`](#admin_revoke_user_session) | `POST /api/admin/users/{user_id}/sessions/{session_id}/revoke` | admin keys | Sign out one of an account's devices (admin) |
| [`admin_create_invite_link`](#admin_create_invite_link) | `POST /api/admin/invite-links` | admin keys | Create an invite link (admin) |
| [`admin_rename_invite_link`](#admin_rename_invite_link) | `POST /api/admin/invite-links/{link_id}/rename` | admin keys | Rename an invite link (admin) |
| [`admin_set_invite_link_limits`](#admin_set_invite_link_limits) | `POST /api/admin/invite-links/{link_id}/limits` | admin keys | Change an invite link's places and expiry (admin) |
| [`admin_switch_on_invite_link`](#admin_switch_on_invite_link) | `POST /api/admin/invite-links/{link_id}/switch-on` | admin keys | Switch an invite link back on (admin) |
| [`admin_delete_invite_link`](#admin_delete_invite_link) | `POST /api/admin/invite-links/{link_id}/delete` | admin keys | Delete an invite link (admin) |
| [`admin_restore_invite_link`](#admin_restore_invite_link) | `POST /api/admin/invite-links/{link_id}/restore` | admin keys | Undo deleting an invite link (admin) |
| [`files_list`](#files_list) | `GET /api/files` | any key | List your files |
| [`files_get`](#files_get) | `GET /api/files/{file_id}` | any key | One of your files |
| [`files_start_upload`](#files_start_upload) | `POST /api/files` | any key | Start uploading a file |
| [`files_finish_upload`](#files_finish_upload) | `POST /api/files/{file_id}/finish` | any key | Finish an upload |
| [`files_download_url`](#files_download_url) | `GET /api/files/{file_id}/download` | any key | Where to download a file |
| [`files_rename`](#files_rename) | `POST /api/files/{file_id}/rename` | any key | Rename a file |
| [`files_delete`](#files_delete) | `POST /api/files/{file_id}/delete` | any key | Delete a file |
| [`files_restore`](#files_restore) | `POST /api/files/{file_id}/restore` | any key | Restore a deleted file |
| [`my_storage`](#my_storage) | `GET /api/account/storage` | any key | Your storage |
| [`admin_list_files`](#admin_list_files) | `GET /api/admin/files` | admin keys | List everyone's files (admin) |
| [`admin_storage`](#admin_storage) | `GET /api/admin/storage` | admin keys | Storage for everyone (admin) |
| [`admin_list_invite_links`](#admin_list_invite_links) | `GET /api/admin/invite-links` | admin keys | List invite links (admin) |
| [`admin_revoke_invite_link`](#admin_revoke_invite_link) | `POST /api/admin/invite-links/{link_id}/revoke` | admin keys | Switch off an invite link (admin) |

### whoami

The account this API key belongs to, the role it acts with (role -- never higher than the key's own scope), and the account's own role.

|  |  |
|---|---|
| Route | `GET /api/me` |
| Read only | yes |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|

### list_team

People with an active account, a page at a time: id and display name. Use the id wherever another tool asks for a user. next is the offset of the following page, or null on the last one.

|  |  |
|---|---|
| Route | `GET /api/team` |
| Read only | yes |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `limit` | integer | no | 1 to 1000 | Rows to return. Default 200. |
| `offset` | integer | no | at least 0 | Rows to skip: the next value of the previous page. |

### admin_list_users

Accounts, a page at a time, including disabled ones, with each one's role and which is the owner. next is the offset of the following page, or null on the last one.

|  |  |
|---|---|
| Route | `GET /api/admin/users` |
| Read only | yes |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `limit` | integer | no | 1 to 1000 | Rows to return. Default 200. |
| `offset` | integer | no | at least 0 | Rows to skip: the next value of the previous page. |

### admin_set_user_disabled

Switch an account off (disabled: true) or back on (false). Disabling signs it out everywhere and voids its live login link; its API keys stop working while it is off. Reversible. The owner and your own account cannot be disabled.

|  |  |
|---|---|
| Route | `POST /api/admin/users/{user_id}/disabled` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `disabled` | boolean | yes |  |  |
| `user_id` | integer | yes | at least 1 | The account's id, from list_team or admin_list_users. |

### admin_revoke_access

For an account that may be compromised: ends every session and voids its live login link and every API key it holds, at once. The account stays enabled; admin_reissue_login lets its owner back in. The owner and your own account cannot be revoked.

|  |  |
|---|---|
| Route | `POST /api/admin/users/{user_id}/revoke-access` |
| Read only | no |
| Destructive | yes |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `user_id` | integer | yes | at least 1 | The account's id, from list_team or admin_list_users. |

### rename_account

Change your own display name (1-80 characters). expect, when given, is the name you think it has now; if someone changed it meanwhile the rename is refused with the current name, so nothing is overwritten unseen.

|  |  |
|---|---|
| Route | `POST /api/account/rename` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `display_name` | string | yes |  |  |
| `expect` | string | no |  | The current name, from whoami. Optional. |

### set_shortcuts

Replace your changed keyboard shortcuts: action name to binding, such as "search": "Mod+K" (Mod is Cmd on a Mac, Ctrl elsewhere). Send the whole set; an empty binding switches a key off, and {} puts every key back to its default. whoami shows the current set.

|  |  |
|---|---|
| Route | `POST /api/account/shortcuts` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `shortcuts` | map of string | yes |  |  |

### set_themes

Replace your whole list of color themes (1 to 40) and pick the one you use. themes: null goes back to following the app's defaults. A theme is the Theme schema in the API reference (config/openapi.json): id, name, mode light|dark, accent, second, and optional colors as #rrggbb. whoami shows the current list.

|  |  |
|---|---|
| Route | `POST /api/account/themes` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `theme_id` | string | no |  |  |
| `themes` | array of object or null | yes |  |  |

### set_theme

Switch to another theme from your list (or the defaults), by its id.

|  |  |
|---|---|
| Route | `POST /api/account/theme` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `theme_id` | string | yes |  |  |

### admin_get_theme_defaults

The theme list everyone without their own list sees. themes is null while the built-in ones apply.

|  |  |
|---|---|
| Route | `GET /api/admin/theme-defaults` |
| Read only | yes |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|

### admin_set_theme_defaults

Replace the theme list everyone without their own list sees, 1 to 40 themes in the Theme schema of the API reference. null puts the built-in themes back.

|  |  |
|---|---|
| Route | `POST /api/admin/theme-defaults` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `themes` | array of object or null | yes |  |  |

### create_my_login_link

A link that signs your own account in on a browser or another device. Only a key with your account's full role may ask: one scoped down to user on an admin account is refused. Answers with login_url (open it to sign in; it works once unless max_uses says otherwise), expires_at and max_uses. Any older link the account could still use stops working; its signed-in devices stay signed in. Revoking or deleting this API key voids the link while it is unused.

|  |  |
|---|---|
| Route | `POST /api/account/login-link` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `expires_in_minutes` | integer | no | at least 0 | How long the link works. Leave it out for the configured default; 0 never expires. |
| `max_uses` | integer | no | at least 0 | How many sign-ins the link allows. Leave it out for once; 0 is unlimited until it expires. |

### cancel_my_login_link

Void your account's live login link without making a new one. Devices it already signed in stay signed in. Answers with removed: 0 or 1.

|  |  |
|---|---|
| Route | `POST /api/account/login-link/cancel` |
| Read only | no |
| Destructive | yes |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|

### admin_create_user

Make a new account and its first login link, to hand to the person. role is admin or user. Answers with user_id and the link. Answers with login_url (open it to sign in; it works once unless max_uses says otherwise), expires_at and max_uses. Any older link the account could still use stops working; its signed-in devices stay signed in. Revoking or deleting this API key voids the link while it is unused.

|  |  |
|---|---|
| Route | `POST /api/admin/users` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `display_name` | string | yes |  | 1-80 characters. |
| `expires_in_minutes` | integer | no | at least 0 | How long the link works. Leave it out for the configured default; 0 never expires. |
| `max_uses` | integer | no | at least 0 | How many sign-ins the link allows. Leave it out for once; 0 is unlimited until it expires. |
| `role` | string | yes | one of `admin`, `user` |  |

### admin_reissue_login

For someone who lost their device or their link: a fresh login link for their account. Answers with login_url (open it to sign in; it works once unless max_uses says otherwise), expires_at and max_uses. Any older link the account could still use stops working; its signed-in devices stay signed in. Revoking or deleting this API key voids the link while it is unused.

|  |  |
|---|---|
| Route | `POST /api/admin/users/{user_id}/reissue-login` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `expires_in_minutes` | integer | no | at least 0 | How long the link works. Leave it out for the configured default; 0 never expires. |
| `max_uses` | integer | no | at least 0 | How many sign-ins the link allows. Leave it out for once; 0 is unlimited until it expires. |
| `user_id` | integer | yes | at least 1 | The account's id, from list_team or admin_list_users. |

### admin_cancel_login_link

Void an account's live login link without making a new one: a link that went somewhere it should not have. Devices it already signed in stay signed in; admin_revoke_access ends those too. Answers with removed: 0 or 1.

|  |  |
|---|---|
| Route | `POST /api/admin/users/{user_id}/cancel-login-link` |
| Read only | no |
| Destructive | yes |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `user_id` | integer | yes | at least 1 | The account's id, from list_team or admin_list_users. |

### admin_rename_user

Change another account's display name (1-80 characters). Only the owner renames an admin. expect, when given, is the name you think it has now; if it changed meanwhile the rename is refused with the current name.

|  |  |
|---|---|
| Route | `POST /api/admin/users/{user_id}/rename` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `display_name` | string | yes |  |  |
| `expect` | string | no |  | The current name, from admin_list_users. Optional. |
| `user_id` | integer | yes | at least 1 | The account's id, from list_team or admin_list_users. |

### admin_set_user_role

Make an account an admin or an ordinary user. Only the owner takes an admin's rights away; the owner is always an admin, and nobody changes their own role.

|  |  |
|---|---|
| Route | `POST /api/admin/users/{user_id}/role` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `role` | string | yes | one of `admin`, `user` |  |
| `user_id` | integer | yes | at least 1 | The account's id, from list_team or admin_list_users. |

### admin_delete_user

Remove an account for good, with its sessions, login link and API keys. Cannot be undone: admin_set_user_disabled is the reversible version. Only the owner deletes an admin; nobody deletes the owner or themselves.

|  |  |
|---|---|
| Route | `POST /api/admin/users/{user_id}/delete` |
| Read only | no |
| Destructive | yes |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `user_id` | integer | yes | at least 1 | The account's id, from list_team or admin_list_users. |

### admin_reset_two_factor

For someone who lost both their phone and their recovery codes: switches their two-factor sign-in off so a login link lets them in again. Only the owner resets an admin's; the owner's is reset from a shell.

|  |  |
|---|---|
| Route | `POST /api/admin/users/{user_id}/reset-two-factor` |
| Read only | no |
| Destructive | yes |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `user_id` | integer | yes | at least 1 | The account's id, from list_team or admin_list_users. |

### list_my_sessions

Where your account is signed in, a page at a time: id, when it started and was last seen, the browser, and the name you gave it. Through an API key no device is current. next is the offset of the following page, or null on the last one.

|  |  |
|---|---|
| Route | `GET /api/account/sessions` |
| Read only | yes |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `limit` | integer | no | 1 to 1000 | Rows to return. Default 200. |
| `offset` | integer | no | at least 0 | Rows to skip: the next value of the previous page. |

### label_my_session

Give one of your devices a name (at most 80 characters; empty clears it). expect, when given, is the name you think it has now.

|  |  |
|---|---|
| Route | `POST /api/account/sessions/{session_id}/label` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `expect` | string | no |  |  |
| `label` | string | yes |  |  |
| `session_id` | string | yes |  | The device's id, from list_my_sessions or admin_list_user_sessions. |

### revoke_my_session

End one of your signed-in sessions; that browser has to sign in again.

|  |  |
|---|---|
| Route | `POST /api/account/sessions/{session_id}/revoke` |
| Read only | no |
| Destructive | yes |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `session_id` | string | yes |  | The device's id, from list_my_sessions or admin_list_user_sessions. |

### revoke_my_other_sessions

End every session your account has: through an API key there is no current device to keep, so every browser signs in again. API keys keep working. Answers with how many were removed.

|  |  |
|---|---|
| Route | `POST /api/account/sessions/revoke-others` |
| Read only | no |
| Destructive | yes |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|

### admin_list_user_sessions

Where an account is signed in: every session, with when it started and was last seen. Addresses are shown to the owner only, and only the owner sees the owner's.

|  |  |
|---|---|
| Route | `GET /api/admin/users/{user_id}/sessions` |
| Read only | yes |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `user_id` | integer | yes | at least 1 | The account's id, from list_team or admin_list_users. |

### admin_revoke_user_session

End one of an account's sessions, rather than all of them with admin_revoke_access.

|  |  |
|---|---|
| Route | `POST /api/admin/users/{user_id}/sessions/{session_id}/revoke` |
| Read only | no |
| Destructive | yes |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `session_id` | string | yes |  | The device's id, from list_my_sessions or admin_list_user_sessions. |
| `user_id` | integer | yes | at least 1 | The account's id, from list_team or admin_list_users. |

### admin_create_invite_link

One link for a group: each person who opens it joins as a new user, up to max_uses people, for days days. Answers with join_url (shown only now) and the link's details. Revoking or deleting this API key switches the link off.

|  |  |
|---|---|
| Route | `POST /api/admin/invite-links` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `days` | integer | yes | 1 to 365 |  |
| `label` | string | yes |  | 1-80 characters; people see it on the join page. |
| `max_uses` | integer | yes | 1 to 1000 |  |

### admin_rename_invite_link

Change the label people see on an invite link's join page (1-80 characters). expect, when given, is the label you think it has now.

|  |  |
|---|---|
| Route | `POST /api/admin/invite-links/{link_id}/rename` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `expect` | string | no |  |  |
| `label` | string | yes |  |  |
| `link_id` | string | yes |  | The id from admin_list_invite_links. |

### admin_set_invite_link_limits

Set how many people the link may take in (never fewer than have joined) and, when days is given, a new expiry counted from now. The link then counts as this key's: revoking the key switches it off.

|  |  |
|---|---|
| Route | `POST /api/admin/invite-links/{link_id}/limits` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `days` | integer | no | 1 to 365 | Leave it out to keep the expiry. |
| `link_id` | string | yes |  | The id from admin_list_invite_links. |
| `max_uses` | integer | yes | 1 to 1000 |  |

### admin_switch_on_invite_link

Undo admin_revoke_invite_link: anyone still holding the link can use it again. The link then counts as this key's: revoking the key switches it off.

|  |  |
|---|---|
| Route | `POST /api/admin/invite-links/{link_id}/switch-on` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `link_id` | string | yes |  | The id from admin_list_invite_links. |

### admin_delete_invite_link

Take an invite link off the list and stop it working. The accounts it made stay. admin_restore_invite_link undoes it for a few minutes.

|  |  |
|---|---|
| Route | `POST /api/admin/invite-links/{link_id}/delete` |
| Read only | no |
| Destructive | yes |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `link_id` | string | yes |  | The id from admin_list_invite_links. |

### admin_restore_invite_link

Bring back an invite link deleted in the last few minutes, working as before. The link then counts as this key's: revoking the key switches it off.

|  |  |
|---|---|
| Route | `POST /api/admin/invite-links/{link_id}/restore` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `link_id` | string | yes |  | The id from admin_list_invite_links. |

### files_list

Your uploaded files, a page at a time: id, name, type, size, state (uploading or ready) and when. next is the offset of the following page, or null on the last one.

|  |  |
|---|---|
| Route | `GET /api/files` |
| Read only | yes |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `limit` | integer | no | 1 to 1000 | Rows to return. Default 200. |
| `offset` | integer | no | at least 0 | Rows to skip: the next value of the previous page. |

### files_get

One file: its details, and for an unfinished upload how many pieces have arrived (received), which is where to resume.

|  |  |
|---|---|
| Route | `GET /api/files/{file_id}` |
| Read only | yes |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `file_id` | string | yes |  | The file's id, from files_list or files_start_upload. |

### files_start_upload

Reserve room for a file and get where to send it. The bytes do not go through MCP: POST each piece of chunk_bytes bytes (the last may be shorter), in order from index 0, as the raw body (Content-Type: application/octet-stream) to chunk_url with {index} filled in and the same Authorization: Bearer header this MCP connection uses. curl is a shell recipe that does all of it. Then call files_finish_upload.

|  |  |
|---|---|
| Route | `POST /api/files` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `name` | string | yes |  | The file's name, as it should be shown. |
| `size` | integer | yes | at least 1 | Exact size in bytes. |
| `type` | string | no |  | Its media type, such as application/pdf. Optional. |

### files_finish_upload

Seal an upload once every piece has arrived; the file is then ready to download. Asked again, it answers the same. If pieces are missing it says how many arrived (received).

|  |  |
|---|---|
| Route | `POST /api/files/{file_id}/finish` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `file_id` | string | yes |  | The file's id, from files_list or files_start_upload. |

### files_download_url

The address of one of your files. GET it over HTTP with the same Authorization: Bearer header this MCP connection uses; the bytes do not come through MCP.

|  |  |
|---|---|
| Route | `GET /api/files/{file_id}/download` |
| Read only | yes |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `file_id` | string | yes |  | The file's id, from files_list or files_start_upload. |

### files_rename

Give one of your files a new name. expect, when given, is the name you think it has now; a rename made meanwhile is refused with the current name.

|  |  |
|---|---|
| Route | `POST /api/files/{file_id}/rename` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `expect` | string | no |  |  |
| `file_id` | string | yes |  | The file's id, from files_list or files_start_upload. |
| `name` | string | yes |  |  |

### files_delete

Move one of your files to the bin. files_restore brings it back for a while; after that it is gone.

|  |  |
|---|---|
| Route | `POST /api/files/{file_id}/delete` |
| Read only | no |
| Destructive | yes |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `file_id` | string | yes |  | The file's id, from files_list or files_start_upload. |

### files_restore

Bring back a file you deleted, while it is still in the bin and your storage quota has room.

|  |  |
|---|---|
| Route | `POST /api/files/{file_id}/restore` |
| Read only | no |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `file_id` | string | yes |  | The file's id, from files_list or files_start_upload. |

### my_storage

How many bytes your files use, and your quota (0 is no limit).

|  |  |
|---|---|
| Route | `GET /api/account/storage` |
| Read only | yes |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|

### admin_list_files

Every account's files, a page at a time, with whose each one is (user_id, user_name). next is the offset of the following page, or null on the last one.

|  |  |
|---|---|
| Route | `GET /api/admin/files` |
| Read only | yes |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `limit` | integer | no | 1 to 1000 | Rows to return. Default 200. |
| `offset` | integer | no | at least 0 | Rows to skip: the next value of the previous page. |

### admin_storage

How many bytes everyone's files use together, and the disk's free and total bytes where the system says.

|  |  |
|---|---|
| Route | `GET /api/admin/storage` |
| Read only | yes |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|

### admin_list_invite_links

Group invite links, a page at a time: label, places used and left, expiry, and whether each still works. The links themselves are never shown again after creation. next is the offset of the following page, or null on the last one.

|  |  |
|---|---|
| Route | `GET /api/admin/invite-links` |
| Read only | yes |
| Destructive | no |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `limit` | integer | no | 1 to 1000 | Rows to return. Default 200. |
| `offset` | integer | no | at least 0 | Rows to skip: the next value of the previous page. |

### admin_revoke_invite_link

Stop an invite link from letting anyone else join or sign in through it. Accounts it already made keep working. Cannot be switched back on.

|  |  |
|---|---|
| Route | `POST /api/admin/invite-links/{link_id}/revoke` |
| Read only | no |
| Destructive | yes |

**Arguments**

| Field | Type | Required | Limits | Description |
|---|---|---|---|---|
| `link_id` | string | yes |  | The id from admin_list_invite_links. |

### HTTP only

Routes an API key may call that have no tool, and why.

| Route | Why |
|---|---|
| `GET /api/mcp.json` | The MCP reference itself; tools/list gives the same list. |
| `GET /api/openapi.json` | The API reference itself; GET /api/mcp.json is the MCP one. |
| `GET /metrics` | Scraped by Prometheus over HTTP. |
| `POST /api/files/{file_id}/chunks/{index}` | File bytes: MCP messages are JSON, and clients cap their size. files_start_upload says where to send each piece, with the same key. |
| `POST /api/logout` | Ends a browser session; a key has none to end. |
| `POST /api/logs/events` | The browser's own activity reports (track.js), not an action. |
| `POST /mcp` | The MCP endpoint itself. |
