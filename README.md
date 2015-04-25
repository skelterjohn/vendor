# vendor
Tool for vendoring repositories.

Currently only supports git.

```
Usage: vendor [-d DIR] -s -|CONFIG  # save
       vendor [-d DIR] -r -|CONFIG  # restore
```

Default for `DIR` is the current working directory.

`vendor -s` searches through `DIR` looking for repositories, making a record of all that it finds, and writes it to `CONFIG`.

`vendor -r` looks at `CONFIG`, and clones all the recorded repositories into `DIR`.

If `CONFIG` is `"-"`, stdin or stdout is used as appropriate.

`vendor` prints the local directories of the repositories it processes.
