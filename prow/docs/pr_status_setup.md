# How to setup PR Status 
This document helps configure [PR Status endpoints](https://prow.k8s.io/pr). 

## Setup secrets
PR status is an OAuth App that query pull requests on behalf of the authenticated users.
Therefore, some secret pieces of information are needed to authorize users for the app. The following
steps will show you how to setup an oauth app that works with PR Status.
1. Create your GitHub Oauth application 

    https://developer.github.com/apps/building-oauth-apps/creating-an-oauth-app/
    
    Make sure to create a GitHub Oauth App and not a regular GitHub App.
    
    The callback url should be:
    
    `<PROW_BASE_URL>/github-login/redirect`
2. Create a secret file for github oauth that has the following content. The information can be found in the [GitHub OAuth developer settings](https://github.com/settings/developers):
    
    ```
    client_id: <APP_CLIENT_ID>
    client_secret: <APP_CLIENT_SECRET>
    redirect_url: <PROW_BASE_URL>/github-login/redirect
    final_redirect_url: <PROW_BASE_URL>/pr
    ```
    
    If Prow is expected to work with private repositories, add
    ```
    scopes:
    - repo
    ```
    
3. Create another secret file for the cookie store. The file should contain a random 64-byte length base64 key. For example, you can use `openssl` to generate the key
    
    ```
    openssl rand -out cookie.txt -base64 64
    ```
4. Use `kubectl`, which should already point to your Prow cluster, to create secrets using the command:
    
    `kubectl create secret generic github-oauth-config --from-file=secret=<PATH_TO_YOUR_GITHUB_SECRET>`

    `kubectl create secret generic cookie --from-file=secret=<PATH_TO_YOUR_COOKIE_KEY_SECRET>`
5. To use the secrets, you can either:

    * [Mount](https://kubernetes.io/docs/concepts/configuration/secret/#using-secrets) secrets to your deck volume:

        Open `test-infra/prow/cluster/deck_deployment.yaml`.
        Under `volumes` token, add:
        ```
        - name: oauth-config
          secret:
              secretName: github-oauth-config
        - name: cookie-secret
          secret:
              secretName: cookie
        ```
        Under `volumeMounts` token, add:
        ```
        - name: oauth-config
          mountPath: /etc/github
          readOnly: true
        - name: cookie-secret
          mountPath: /etc/cookie
          readOnly: true
        ```
    * Or, pass the path to your secrets to `deck` using the `--github-oauth-config-file`  and `--cookie-secret` flags.

6. Set the flag `--oauth-url=/github-login` on the deck deployment.

## Run PR Status endpoint locally
Firstly, you will need a GitHub OAuth app. Please visit step 1 - 3 above. 

When testing locally, pass the path to your secrets to `deck` using the `--github-oauth-config-file`  and `--cookie-secret` flags.

Run the commands:

`go build . && ./deck --config-path=../../config.yaml --github-oauth-config-file=<PATH_TO_YOUR_GITHUB_OAUTH_SECRET> --cookie-secret=<PATH_TO_YOUR_COOKIE_SECRET> --oauth-url=/pr`

## Run PR Status endpoint on a test cluster
If hosting your instance on http instead of https, you will need to set `Secure` to `false` every time it appears in [`githuboauth.go`](/prow/githuboauth/githuboauth.go).  
There is an open issue for a flag that will do this for you: [#12989](https://github.com/kubernetes/test-infra/issues/12989)
