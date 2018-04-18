  The [`gx`](https://github.com/whyrusleeping/gx) project is a powerful package manager built on top of [IPFS](https://ipfs.io/). It is highly performant, fully decentralized and incorruptible. The goal of `ungx` is to get rid of it 😇

## Why?!

As powerful as `gx` is, it doesn't yet play nice with non-gx workflows:

 * You can't `go get` a `gx` based project, as the dependencies are hosted via IPFS, so special tooling support is needed to interpret the import paths and also to retrieve their content. This also means no other Go project can depend on your `gx` based package.
 * The import paths in `gx` based packages lose all canonical URL context and get replaced with random strings of letters and numbers (e.g. `gx/ipfs/QmPXvegq26x982cQjSfbTvSzZXn7GiaMwhhVPHkeTEhrPT/sys`). [These will make your head spin](https://github.com/ipfs/go-ipfs/blob/master/core/core.go#L42). It will also make `goimports`' head spin when it realises there are 15 versions of `sys`under different hashes.
 * Finally, if you vendor in a `gx` based package with all its juicy hash-based import paths and use these in your own code too, you'll be very unhappy at the first occasion when you need to update dependencies and realize you have to update **all** the import statements in your code with new hashes, that you won't even know yourself.

TL;DR `gx` is an amazing project, but until the Go ecosystem builds a tool that can bridge the two worlds, I need a way to use `gx` projects without forcing me to switch over to `gx` myself.

## How?

With the *why* out of the way, lets see *how* `ungx` helps make our lives easier. The goal of `ungx` is to take a `gx` based package/repository, resolve all the dependencies in it via `gx` and then rewrite/vendor all the dependencies into legacy Go style.

It's operation is fairly simplistic:

 * Run `gx install --local` to fetch all `gx` dependencies and vendor them in with hashes.
 * Find all `gx` dependencies that do not have multiple versions (we can't rewrite clashes).
 * Vendor all non-clashing plain Go dependencies under `vendor` with their canonical path.
 * Embed all non-clashing `gx` dependencies under `gxdeps` with their canonical path.
 * Rewrite all import statements for all non-clashing dependencies to the new paths.
 * Optionally rewrite the root import path to a custom one specified via `--fork`.

**Note, it will overwrite your original checked out repo!**

## Example

If we'd want to make a nice `go-ipfs` fork that doesn't contain strange import paths and plays nice with existing Go toolings, we could use `ungx` for it:

First up we need the original `go-ipfs` repo.

```
$ go get -u -d github.com/ipfs/go-ipfs
```

Then we need `gx` for dependency retrieval and `ungx` for rewrites:

```
$ go get -u github.com/whyrusleeping/gx
$ go get -u github.com/karalabe/ungx
```

Finally we can let `ungx` do its magic:

```
$ cd $GOPATH/github.com/ipfs/go-ipfs
$ ungx

2018/04/13 17:27:02 Vendoring in gx dependencies
[done] [fetch]   go-libp2p-secio               QmT8TkDNBDyBsnZ4JJ2ecHU7qN184jkw1tY8y4chFfeWsy 835ms
[done] [fetch]   go-log                        QmRb5jh8z2E8hMGN2tkvs1yHynUanqnZ3UeKwgN1i9P1F8 834ms
[done] [fetch]   goleveldb                     QmbBhyDKsY4mbY6xsKt3qu9Y7FPvMJ6qbD8AMjYYvPRw1g 507ms
[...]
[done] [install] opentracing-go                QmWLWmRVSiagqP15jczsGME1qpob6HDbtbHAY2he9W5iUo 0s
[done] [install] go-fs-lock                    QmPdqSMmiwtQCBC515gFtMW2mP14HsfgnyQ2k5xPQVxMge 8ms
[done] [install] go-bitfield                   QmTbBs3Y3u5F69XNJzdnnc6SP5GKgcXxCDzx6w8m6piVRT 4ms

2018/04/13 17:28:36 Rewriting gx/ipfs/QmNeSwALyTCrgtCTsPiF7tcDN6uLtdi8qCMtFm7nct1nm1/httprouter to github.com/julienschmidt/httprouter
2018/04/13 17:28:37 Rewriting gx/ipfs/QmQFhPsJCp82az4SXbziP9QcVSqggEELnV9wGZqMR1EfMB/go-smux-spdystream to github.com/whyrusleeping/go-smux-spdystream
2018/04/13 17:28:37 Rewriting gx/ipfs/QmT8TkDNBDyBsnZ4JJ2ecHU7qN184jkw1tY8y4chFfeWsy/go-libp2p-secio to github.com/libp2p/go-libp2p-secio
[...]
2018/04/13 17:28:59 Rewriting gx/ipfs/QmTEmsyNnckEq8rEfALfdhLHjrEHGoSGFDrAYReuetn7MC/go-net to golang.org/x/go-net
2018/04/13 17:28:59 Rewriting gx/ipfs/QmVYxfoJQiZijTgPNHCHgHELvQpbsJNTg6Crmc3dQkj3yy/golang-lru to github.com/hashicorp/golang-lru
2018/04/13 17:28:59 Rewriting gx/ipfs/QmZyZDi491cCNTLfAhwcaDii2Kg4pwKRkhqQzURGDvY6ua/go-multihash to github.com/multiformats/go-multihash
```

And voila, we have a fork of `go-ipfs` that does not contain cryptic hash import paths and is a joy to work with. If you want to update your fork to a new version, repeat the above procedure in a pristine GOPATH and overwrite your old fork with the newly generated one.

*Note, if you want to publish your dependency publicly, you'll need to rewrite all the package's internal imports to your fork paths (e.g. `ungx --fork=github.com/myipfs/go-ipfs`). and manually move the repository contents to `$GOPATH/github.com/myipfs/go-ipfs`.*

## Disclaimer

This tool is a toy. I built it for my personal hobby projects. You're welcome to use it, but don't expect support, stability or even responses from me 😋
