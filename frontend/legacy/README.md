# legacy/ — retired vanilla page

This directory is the **reference implementation** the React app replaces. It is
a byte-for-byte copy of `frontend/` at `f4d99b7`, with exactly one change: the
paths of `tests/proposal-readback.test.cjs` were rewritten to resolve from this
one-level-deeper location (`../../` instead of `../`). No assertion was added,
removed or altered.

It exists so the 60 assertions of the old VM harness stay runnable while
`tests/**` is rebuilt in the React app:

```sh
npm run test:legacy
```

`N9` in `odd/urgent/rediseno-pagina/app-react-vite.md` deletes this directory
only after every one of those assertions has been ported and the port shows zero
lost identifiers.
