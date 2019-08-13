# CSRF attacks
In Deck, we make a number of `POST` requests that require user authentication. These requests are susceptible
to [cross site request forgery (CSRF) attacks](https://en.wikipedia.org/wiki/Cross-site_request_forgery), 
in which a malicious actor tricks an already authenticated user into submitting a form to one of these endpoints 
and performing one of these protected actions on their behalf. 

# Protection
If `--cookie-secret` is 32 or more bytes long, CSRF protection is automatically enabled.
If `--rerun-creates-job` is specified, CSRF protection is required, and accordingly, 
`--cookie-secret` must be 32 bytes long. 

We protect against CSRF attacks using the [gorilla CSRF](https://github.com/gorilla/csrf) library, implemented 
in [#13323](https://github.com/kubernetes/test-infra/pull/13323). Broadly, this protection works by ensuring that 
any `POST` request originates from our site, rather than from an outside link. 
We do so by requiring that every `POST` request made to Deck includes a secret token either in the request header 
or in the form itself as a hidden input. 

We cryptographically generate the CSRF token using the `--cookie-secret` and a user session value and 
include it as a header in every `POST` request made from Deck. 
If you are adding a new `POST` request, you must include the CSRF token as described in the gorilla 
[documentation](https://github.com/gorilla/csrf).

The gorilla library expects a 32-byte CSRF token. If `--cookie-secret` is sufficiently long, 
direct job reruns will be enabled via the `/rerun` endpoint. Otherwise, if `--cookie-secret` is less 
than 32 bytes and `--rerun-creates-job` is enabled, Deck will refuse to start. Longer values will 
work but should be truncated. 

By default, gorilla CSRF requires that all `POST` requests are made over HTTPS. If developing locally
over HTTP, you must specify `--allow-insecure` to Deck, which will configure both gorilla CSRF 
and GitHub oauth to allow HTTP requests. 

CSRF can also be executed by tricking a user into making a state-mutating `GET` request. All 
state-mutating requests must therefore be `POST` requests, as gorilla CSRF does not secure `GET`
requests.
