# Security

**Table of Contents:**
- [Reporting Security Vulnerabilities](#reporting-security-vulnerabilities)
- [Security Features](#security-features)
  - [SSL Support](#ssl-support)
  - [Token-Based Authentication](#token-based-authentication)
    - [Use with auto-generated tokens](#use-with-auto-generated-tokens)
    - [Specify your own token](#specify-your-own-token)
    - [Use Token Authentication with SSL enabled](#use-token-authentication-with-ssl-enabled)
    - [Token Recovery](#token-recovery)
    - [How to disable Token-Based Authentication](#how-to-disable-token-based-authentication)
- [Security Best Practices](#security-best-practices)

## Reporting Security Vulnerabilities
If you believe you have discovered a security vulnerability, please report it to
security@mathworks.com instead of GitHub. Please see
[MathWorks Vulnerability Disclosure Policy for Security Researchers](https://www.mathworks.com/company/aboutus/policies_statements/vulnerability-disclosure-policy.html)
for additional information.

----

## Security Features
The following features are available in `matlab-proxy` to provide secure access to MATLAB:
  - [SSL Support](#ssl-support)
  - [Token-Based Authentication](#token-based-authentication)

----

## SSL Support

1. *MWI_ENABLE_SSL*

    Use the environment variable `MWI_ENABLE_SSL` to configure SSL/TLS support. To enable SSL/TLS, set `MWI_ENABLE_SSL` to `true`. By default, this uses a self-signed certificate. To use custom SSL certificates instead, specify these files using the following environment variables when you start the server.

2. *MWI_SSL_CERT_FILE*

    A string with the full path to a single file in PEM format containing the certificate as well as any number of CA certificates needed to establish the certificate's authenticity.

3. *MWI_SSL_KEY_FILE*

   A string with the full path to a file containing the private key. If absent, the private key must be present in the cert file provided using `MWI_SSL_CERT_FILE`.

Example:
```bash
# Start with SSL enabled
env MWI_ENABLE_SSL=true MWI_SSL_CERT_FILE="/path/to/certificate.pem" MWI_SSL_KEY_FILE="/path/to/keyfile.key" ./matlab-proxy

# The access link appears in the terminal at startup:
Access MATLAB at: https://localhost:8080/?mwi-auth-token=...

# NOTE: The server is running HTTPS
```

----

## Token-Based Authentication

`matlab-proxy` is a web server that allows one to start and access MATLAB on the machine the server is running on. Anyone with access to the server can access MATLAB and thereby the machine on which it is running.

`Token-Based Authentication` is enabled by default and the server requires a token to authenticate access. Users can provide this token to the server in the following ways:

1. Use the URL query parameter `mwi-auth-token`. Example:
    ```
    https://localhost:8080/?mwi-auth-token=abcdef...
    ```
    The server sets a session cookie after initial authentication. All subsequent requests from the same browser use the cookie automatically.

2. Use the auth token input field that appears when accessing the server without a token and without a valid session cookie.

3. Use a `mwi-auth-token` HTTP header. Example:
    ```
    mwi-auth-token: abcdef...
    ```

**NOTE**: It is highly recommended to use this feature along with SSL enabled as shown [here](#use-token-authentication-with-ssl-enabled).

### Use with auto-generated tokens

When enabled, the server generates a random URL-safe token and prints the access URL at startup:

```bash
# Start with Token-Based Authentication enabled (the default)
./matlab-proxy

# The access link appears in the terminal:
Access MATLAB at: http://localhost:8080/?mwi-auth-token=SY78vUw5qyf0JTJzGK4mKJlk_exkzL_SMFJyilbGtNI
```

In this example `SY78vUw5qyf0JTJzGK4mKJlk_exkzL_SMFJyilbGtNI` is the token that the server needs for future communication.

After initial access, a session cookie is set by the browser. All subsequent access from the same browser to the server will not require this token. You will however need this token to access the server from a new browser session, an incognito window, or if you have cleared cookies.

### Specify your own token

Optionally, you can specify your own secret token using the environment variable `MWI_AUTH_TOKEN`.
Ensure that your custom token is URL safe.
A token can safely contain any combination of alphanumeric text along with the following permitted characters: `- . _ ~`

See [URI Specification RFC3986](https://www.ietf.org/rfc/rfc3986.txt) for more information on URL safe characters.

Example:
```bash
# Start with a custom token
env MWI_AUTH_TOKEN=MyCustomSecretToken ./matlab-proxy

# The access link appears in the terminal:
Access MATLAB at: http://localhost:8080/?mwi-auth-token=MyCustomSecretToken
```

### Use Token Authentication with SSL enabled

It is recommended to enable both `Token-Based Authentication` and `SSL` to secure your access to MATLAB. The following command enables access to MATLAB using HTTPS and token-based authentication:

```bash
env MWI_ENABLE_SSL=true MWI_SSL_CERT_FILE="/path/to/certificate.pem" MWI_SSL_KEY_FILE="/path/to/keyfile.key" MWI_AUTH_TOKEN=asdf ./matlab-proxy

# The access link appears in the terminal:
Access MATLAB at: https://localhost:8080/?mwi-auth-token=asdf
```

### Token Recovery

To recover the token for a previously started server, you need access to the machine on which the server was started, logged in as the user that started the server.

```bash
# List all running servers (tokens are included in the URLs)
./matlab-proxy --list

  MATLAB Proxy Servers

+--------------------+------------+----------------------+----------------------------------------------------+
| Created On         | MATLAB Ver | Session Name         | Server URL                                         |
+--------------------+------------+----------------------+----------------------------------------------------+
| 18/03/26 19:52:10  | R2025b     |                      | http://localhost:43777/?mwi-auth-token=2b711f3a... |
+--------------------+------------+----------------------+----------------------------------------------------+

# URLs only (for scripting)
./matlab-proxy --list --quiet
```

For servers with Token-Based Authentication enabled, the URLs above will include their tokens.

### How to disable Token-Based Authentication

Since `Token-Based Authentication` is enabled by default, set the environment variable `MWI_ENABLE_TOKEN_AUTH` to `false` on server startup to disable it.

Example:
```bash
# Start with Token-Based Authentication disabled
env MWI_ENABLE_TOKEN_AUTH=false ./matlab-proxy

# The access link appears in the terminal:
Access MATLAB at: http://localhost:8080/
```

## Security Best Practices

* **Never share access to your server.** Never share URLs from `matlab-proxy` with others. Doing so is equivalent to sharing your user account.

* **System administrators** who start `matlab-proxy` for other users must start the proxy as the user for whom the server is intended.

* **Use SSL in production.** Without SSL, tokens are transmitted in plaintext over the network. Always enable `MWI_ENABLE_SSL` when the server is accessible over a network.

* **Use unique ports or separate hosts** when running multiple instances. Session cookies are scoped by port (`mwi-auth-session-<port>`), so instances on different ports are isolated. However, if auth is disabled, any browser on the network can access the server.

---

Copyright 2026 The MathWorks, Inc.
