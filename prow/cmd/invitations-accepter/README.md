# Invitations Accepter

The `invitations-accepter` tool approves all pending repository invitations. 

## Usage

*example*:
```sh
invitations-accepter --dry-run=false --github-token-path=/etc/github/oauth
```

using with GitHub Apps

```sh
invitations-accepter --dry-run=false --github-app-id=12345 --github-app-private-key-path=/etc/github/cert

```