# Subfilter

Subfilter is a middleware plugin for [Traefik][traefik] which rewrites the HTTP response body
by replacing a search regex by a replacement string. Subfilter was directly cloned from
[plugin-rewritebody][rewritebody] and modified (keeping all git history) to address issues not resolved in the 
upstream repository

## Configuration

### Static

```toml
[pilot]
  token = "xxxx"

[experimental.plugins.subfilter]
  modulename = "github.com/DirtyCajunRice/traefik-subfilter-plugin"
  version = "v0.4.0"
```

### Dynamic

To configure the `Subfilter` plugin, create a middleware in your configuration as explained [here][middleware-docs].
The following examples create and use `subfilter` to replace all foo occurrences by bar in the HTTP response body.

If you want to apply some limits on the response body, you can chain this middleware plugin with
the [Buffering middleware][buffering-middleware] from Traefik.

```toml
[http.routers]
  [http.routers.my-router]
    rule = "Host(`localhost`)"
    middlewares = ["subfilter-foo"]
    service = "my-service"

[http.middlewares]
  [http.middlewares.subfilter-foo.plugin.subfilter]
    # Keep Last-Modified header returned by the HTTP service.
    # By default, the Last-Modified header is removed.
    lastModified = true

    # Rewrites all "foo" occurences by "bar"
    [[http.middlewares.subfilter-foo.plugin.subfilter.filters]]
      regex = "foo"
      replacement = "bar"

[http.services]
  [http.services.my-service]
    [http.services.my-service.loadBalancer]
      [[http.services.my-service.loadBalancer.servers]]
        url = "http://127.0.0.1"
```

### Dynamic - Kubernetes

extraArgs

```yaml
...
- "--experimental.plugins.subfilter.modulename=github.com/DirtyCajunRice/traefik-subfilter-plugin"
- "--experimental.plugins.subfilter.version=v0.4.0"
...
```

Middleware CRD

```yaml
apiVersion: traefik.containo.us/v1alpha1
kind: Middleware
metadata:
  name: subfilter-foo
spec:
  plugin:
    subfilter:
      lastModified: true
      filters:
        - regex: foo
          replacement: bar
```

### My Regex Fails!

Subfilter uses golang's [regexp][regexp] package. You can use [The Go Playground][playground] to test your regex.

Here is a minimally viable example:

```go
package main

import (
	"fmt"
	"regexp"
)

func main() {
	r := regexp.MustCompile(`((href|src)=")/`)
	body := []byte("<html><head><link rel=\"stylesheet\" href=\"/style.css\"></head></html>")
	replace :=[]byte("${1}/subdomain/")
	fmt.Printf("%s\n", r.ReplaceAll(body, replace))
}
```

[traefik]: https://github.com/traefik/traefik

[middleware-docs]: https://docs.traefik.io/middlewares/overview/

[buffering-middleware]: https://docs.traefik.io/middlewares/buffering/

[rewritebody]: https://github.com/traefik/plugin-rewritebody

[regexp]: https://golang.org/pkg/regexp/

[playground]: https://play.golang.org/
