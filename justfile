# pushes all changes to the main branch
push +COMMIT_MESSAGE:
  git add .
  git commit -m "{{COMMIT_MESSAGE}}"
  git pull origin main
  git push origin main

tag +TAG_NAME:
  git tag {{TAG_NAME}}
  git push origin {{TAG_NAME}}

generate:
  go generate ./...

test:
  go test -race ./...

air:
  air -c .air.toml