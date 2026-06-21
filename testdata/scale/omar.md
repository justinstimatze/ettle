name: omar
role: backend

Upgrading the test framework from the old runner to the new one across the
service repos. It's mechanical but wide — every `*_test.go` gets the import swap
and a few assertion-helper renames. CI config changes slightly to call the new
runner. I'm scripting most of it with a codemod and babysitting the diff.

No product behavior changes, no config-file edits beyond the CI YAML. Hoping to
have it merged by end of week so we stop maintaining two test styles.
