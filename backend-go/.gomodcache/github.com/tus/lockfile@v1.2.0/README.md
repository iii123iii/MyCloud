lockfile
=========
Handle locking via pid files.

*Attention:* This is a fork of [Ingo Oeser's amazing work](https://github.com/nightlyone/lockfile) whose behavior differs a bit. While the original package allows a process to obtain the same lock twice, this fork forbids this behavior.

![Build Status](https://github.com/tus/lockfile/actions/workflows/ci.yml/badge.svg)

Install
-------

Install one of the [two latest major releases](https://go.dev/dl/) of Go. Then run

```
go get github.com/tus/lockfile
```

Documentation
-------------

[Package documentation at pkg.go.dev](https://pkg.go.dev/github.com/tus/lockfile).

Contributing
------------

Contributions are welcome. Please open an issue or send me a pull request for a dedicated branch.

License
-------
MIT
