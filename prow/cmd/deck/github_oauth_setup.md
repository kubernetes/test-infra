# How to setup GitHub Oauth
This document helps configure GitHub Oauth, which is required for [PR Status](https://prow.k8s.io/pr)
and for the rerun button on [Prow Status](https://prow.k8s.io). 
If OAuth is configured, Prow will perform GitHub actions on behalf of the authenticated users.
This is necessary to fetch information about pull requests for the PR Status page and to 
authenticate users when checking if they have permission to rerun jobs via the rerun button on Prow Status.

## Set up secrets
The following steps will show you how to set up an OAuth app.
1. Create your GitHub Oauth application 

    https://developer.github.com/apps/building-oauth-apps/creating-an-oauth-app/
    
    Make sure to create a GitHub Oauth App and not a regular GitHub App.
    
    The callback url should be:
    
    `<PROW_BASE_URL>/github-login/redirect`
2. Create a secret file for GitHub OAuth that has the following content. The information can be found in the [GitHub OAuth developer settings](https://github.com/settings/developers):
    
    ```yaml
    client_id: <APP_CLIENT_ID>
    client_secret: <APP_CLIENT_SECRET>
    redirect_url: <PROW_BASE_URL>/github-login/redirect
    final_redirect_url: <PROW_BASE_URL>/pr
    ```
    
    If Prow is expected to work with private repositories, add
    ```yaml
    scopes:
    - repo
    ```
    
3. Create another secret file for the cookie store. This cookie secret will also be used for CSRF protection.
  The file should contain a random 32-byte length base64 key. For example, you can use `openssl` to generate the key
    
    ```
    openssl rand -out cookie.txt -base64 32
    ```
4. Use `kubectl`, which should already point to your Prow cluster, to create secrets using the command:
    
    `kubectl create secret generic github-oauth-config --from-file=secret=<PATH_TO_YOUR_GITHUB_SECRET>`

    `kubectl create secret generic cookie --from-file=secret=<PATH_TO_YOUR_COOKIE_KEY_SECRET>`
5. To use the secrets, you can either:

    * [Mount](https://kubernetes.io/docs/concepts/configuration/secret/#using-secrets) secrets to your deck volume:

        Open `test-infra/config/prow/cluster/deck_deployment.yaml`.
        Under `volumes` token, add:
        ```yaml
        - name: oauth-config
          secret:
              secretName: github-oauth-config
        - name: cookie-secret
          secret:
              secretName: cookie
        ```
        Under `volumeMounts` token, add:
        ```yaml
        - name: oauth-config
          mountPath: /etc/githuboauth
          readOnly: true
        - name: cookie-secret
          mountPath: /etc/cookie
          readOnly: true
        ```
    * Add the following flags to `deck`:
      ```yaml
      - --github-oauth-config-file=/etc/githuboauth/secret
      - --oauth-url=/github-login
      ```
      Note that the `--oauth-url` should eventually be changed to a boolean as described 
      in [#13804](https://github.com/kubernetes/test-infra/issues/13804).
    * You can also set your own path to the cookie secret using the `--cookie-secret` flag.
    * To prevent `deck` from making mutating GitHub API calls, pass in the `--dry-run` flag.

## Using A GitHub bot
The rerun button can be configured so that certain GitHub teams are allowed to trigger certain jobs
from the frontend. In order to make API calls to determine whether a user is on a given team, `deck` needs 
to use the access token of an org member. 

If not, you can create a new GitHub account, make it an org number, and set up a personal access token 
[here](https://github.com/settings/tokens).

Then create the access token secret:

`kubectl create secret generic oauth-token --from-file=secret=<PATH_TO_ACCESS_TOKEN>`

Add the following to `volumes` and `volumeMounts`:
```yaml
volumeMounts:
- name: oauth-token
  mountPath: /etc/github
  readOnly: true
volumes:
- name: oauth-token
  secret:
      secretName: oauth-token
```

Pass the file path to `deck` as a flag:

`--github-token-path=/etc/github/oauth`

You can optionally use [ghproxy](https://github.com/kubernetes/test-infra/blob/master/ghproxy/README.md) to reduce token usage. 

## Run PR Status endpoint locally
Firstly, you will need a GitHub OAuth app. Please visit step 1 - 3 above. 

When testing locally, pass the path to your secrets to `deck` using the `--github-oauth-config-file`  and `--cookie-secret` flags.

Run the command:

`go build . && ./deck --config-path=../../../config/prow/config.yaml --github-oauth-config-file=<PATH_TO_YOUR_GITHUB_OAUTH_SECRET> --cookie-secret=<PATH_TO_YOUR_COOKIE_SECRET> --oauth-url=/pr`

Or, if you'd like to use bazel, run the command: 

`bazel run //prow/cmd/deck -- --config-path=/absolute/path/to/config.yaml --github-oauth-config-file=<PATH_TO_YOUR_GITHUB_OAUTH_SECRET> --cookie-secret=<PATH_TO_YOUR_COOKIE_SECRET> --oauth-url=/pr`

## Using a test cluster
If hosting your test instance on http instead of https, you will need to use the `--allow-insecure` flag in `deck`.
