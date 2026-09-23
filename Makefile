.PHONY: fmt layout test race cover vet build ts-install ts-check ts-test npm-pack package-smoke version-check reproducible certify release-check verify

fmt:
	find . -type f -name '*.go' \
		-not -path './.git/*' -not -path './vendor/*' -not -path './node_modules/*' \
		-print0 | xargs -0 gofmt -w

layout:
	go test ./test -run TestRepositoryLayout

test:
	go test ./...

race:
	go test -race ./...

cover:
	go test -cover ./...

vet:
	go vet ./...

build:
	go build ./...

ts-install:
	npm ci --prefix sdk/typescript

ts-check:
	npm run check --prefix sdk/typescript

ts-test:
	npm test --prefix sdk/typescript

npm-pack:
	npm run pack:check --prefix sdk/typescript

package-smoke:
	scripts/package_smoke.sh

version-check:
	scripts/verify_versions.sh

reproducible:
	scripts/check_reproducible.sh

certify:
	scripts/certify.sh

release-check:
	scripts/release_dry_run.sh

verify: fmt layout test race cover vet build ts-install ts-check ts-test npm-pack package-smoke version-check reproducible
