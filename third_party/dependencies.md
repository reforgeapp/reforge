# Dependency inventory

Generated from the Go module graph, npm lockfile, and pinned agent skill sources.
Go package scope comes from
`go list -deps` for shipped binaries and `go list -deps -test ./...`; npm scope
comes from each locked package’s production/development install role. Exact-root
licence and notice files are copied to `third-party-notices.txt` with trailing whitespace normalized. The Go
licence name is not inferred from copyright text.

| Dependency | Version | Scope | Licence evidence |
| --- | --- | --- | --- |
| cel.dev/expr | v0.15.0 | module graph/build download only | LICENSE |
| cloud.google.com/go | v0.116.0 | runtime binary; Go tests | LICENSE |
| cloud.google.com/go/auth | v0.9.3 | runtime binary; Go tests | LICENSE |
| cloud.google.com/go/auth/oauth2adapt | v0.2.4 | module graph/build download only | LICENSE |
| cloud.google.com/go/compute/metadata | v0.5.0 | runtime binary; Go tests | LICENSE |
| cloud.google.com/go/iam | v1.2.0 | module graph/build download only | LICENSE |
| cloud.google.com/go/longrunning | v0.5.6 | module graph/build download only | LICENSE |
| cloud.google.com/go/storage | v1.43.0 | module graph/build download only | LICENSE |
| cloud.google.com/go/translate | v1.10.3 | module graph/build download only | LICENSE |
| github.com/BurntSushi/toml | v0.3.1 | module graph/build download only | COPYING |
| github.com/anthropics/anthropic-sdk-go | v1.74.0 | runtime binary; Go tests | LICENSE |
| github.com/aws/aws-sdk-go-v2 | v1.47.0 | runtime binary; Go tests | LICENSE.txt, NOTICE.txt |
| github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream | v1.6.3 | module graph/build download only | LICENSE.txt |
| github.com/aws/aws-sdk-go-v2/config | v1.33.5 | runtime binary; Go tests | LICENSE.txt |
| github.com/aws/aws-sdk-go-v2/credentials | v1.20.5 | runtime binary; Go tests | LICENSE.txt |
| github.com/aws/aws-sdk-go-v2/feature/ec2/imds | v1.20.0 | runtime binary; Go tests | LICENSE.txt |
| github.com/aws/aws-sdk-go-v2/internal/configsources | v1.5.3 | runtime binary; Go tests | LICENSE.txt |
| github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 | v2.8.3 | runtime binary; Go tests | LICENSE.txt |
| github.com/aws/aws-sdk-go-v2/internal/ini | v1.8.0 | module graph/build download only | LICENSE.txt |
| github.com/aws/aws-sdk-go-v2/internal/v4a | v1.5.3 | runtime binary; Go tests | LICENSE.txt |
| github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding | v1.13.19 | runtime binary; Go tests | LICENSE.txt |
| github.com/aws/aws-sdk-go-v2/service/internal/presigned-url | v1.14.3 | runtime binary; Go tests | LICENSE.txt |
| github.com/aws/aws-sdk-go-v2/service/kms | v1.61.0 | runtime binary; Go tests | LICENSE.txt |
| github.com/aws/aws-sdk-go-v2/service/signin | v1.10.0 | runtime binary; Go tests | LICENSE.txt |
| github.com/aws/aws-sdk-go-v2/service/sso | v1.38.0 | runtime binary; Go tests | LICENSE.txt |
| github.com/aws/aws-sdk-go-v2/service/ssooidc | v1.43.0 | runtime binary; Go tests | LICENSE.txt |
| github.com/aws/aws-sdk-go-v2/service/sts | v1.51.0 | runtime binary; Go tests | LICENSE.txt |
| github.com/aws/smithy-go | v1.28.1 | runtime binary; Go tests | LICENSE, NOTICE |
| github.com/bahlo/generic-list-go | v0.2.0 | runtime binary; Go tests | LICENSE |
| github.com/buger/jsonparser | v1.1.2 | runtime binary; Go tests | LICENSE |
| github.com/bytedance/gopkg | v0.1.3 | module graph/build download only | LICENSE |
| github.com/bytedance/sonic | v1.15.0 | module graph/build download only | LICENSE |
| github.com/bytedance/sonic/loader | v0.5.0 | module graph/build download only | LICENSE |
| github.com/census-instrumentation/opencensus-proto | v0.4.1 | module graph/build download only | LICENSE |
| github.com/cespare/xxhash/v2 | v2.3.0 | module graph/build download only | LICENSE.txt |
| github.com/client9/misspell | v0.3.4 | module graph/build download only | LICENSE |
| github.com/cloudwego/base64x | v0.1.6 | module graph/build download only | LICENSE, LICENSE-APACHE |
| github.com/cncf/udpa/go | v0.0.0-20191209042840-269d4d468f6f | module graph/build download only | LICENSE |
| github.com/cncf/xds/go | v0.0.0-20240423153145-555b57ec207b | module graph/build download only | LICENSE |
| github.com/coreos/go-oidc/v3 | v3.21.0 | runtime binary; Go tests | LICENSE, NOTICE |
| github.com/creack/pty | v1.1.24 | module graph/build download only | LICENSE |
| github.com/davecgh/go-spew | v1.1.1 | module graph/build download only | LICENSE |
| github.com/dlclark/regexp2 | v1.11.0 | module graph/build download only | LICENSE |
| github.com/dnaeon/go-vcr | v1.2.0 | module graph/build download only | LICENSE |
| github.com/eliben/go-sentencepiece | v0.7.0 | module graph/build download only | LICENSE |
| github.com/envoyproxy/go-control-plane | v0.12.1-0.20240621013728-1eb8caab5155 | module graph/build download only | LICENSE |
| github.com/envoyproxy/protoc-gen-validate | v1.0.4 | module graph/build download only | LICENSE |
| github.com/felixge/httpsnoop | v1.0.4 | module graph/build download only | LICENSE.txt |
| github.com/gabriel-vasile/mimetype | v1.4.12 | runtime binary; Go tests | LICENSE |
| github.com/gin-contrib/sse | v1.1.0 | runtime binary; Go tests | LICENSE |
| github.com/gin-gonic/gin | v1.12.0 | runtime binary; Go tests | LICENSE |
| github.com/go-jose/go-jose/v4 | v4.1.4 | runtime binary; Go tests | LICENSE |
| github.com/go-logr/logr | v1.4.2 | module graph/build download only | LICENSE |
| github.com/go-logr/stdr | v1.2.2 | module graph/build download only | LICENSE |
| github.com/go-playground/assert/v2 | v2.2.0 | module graph/build download only | LICENSE |
| github.com/go-playground/locales | v0.14.1 | runtime binary; Go tests | LICENSE |
| github.com/go-playground/universal-translator | v0.18.1 | runtime binary; Go tests | LICENSE |
| github.com/go-playground/validator/v10 | v10.30.1 | runtime binary; Go tests | LICENSE |
| github.com/goccy/go-json | v0.10.5 | module graph/build download only | LICENSE |
| github.com/goccy/go-yaml | v1.19.2 | runtime binary; Go tests | LICENSE |
| github.com/golang/glog | v1.2.1 | module graph/build download only | LICENSE |
| github.com/golang/groupcache | v0.0.0-20210331224755-41bb18bfe9da | runtime binary; Go tests | LICENSE |
| github.com/golang/mock | v1.1.1 | module graph/build download only | LICENSE |
| github.com/golang/protobuf | v1.5.4 | module graph/build download only | LICENSE |
| github.com/golang/snappy | v0.0.4 | module graph/build download only | LICENSE |
| github.com/google/go-cmp | v0.7.0 | runtime binary; Go tests | LICENSE |
| github.com/google/go-pkcs11 | v0.3.0 | module graph/build download only | LICENSE |
| github.com/google/gofuzz | v1.0.0 | module graph/build download only | LICENSE |
| github.com/google/jsonschema-go | v0.4.2 | module graph/build download only | LICENSE |
| github.com/google/martian/v3 | v3.3.3 | module graph/build download only | LICENSE |
| github.com/google/s2a-go | v0.1.8 | runtime binary; Go tests | LICENSE.md |
| github.com/google/uuid | v1.6.0 | module graph/build download only | LICENSE |
| github.com/googleapis/enterprise-certificate-proxy | v0.3.4 | runtime binary; Go tests | LICENSE |
| github.com/googleapis/gax-go/v2 | v2.13.0 | module graph/build download only | LICENSE |
| github.com/gorilla/websocket | v1.5.3 | runtime binary; Go tests | LICENSE |
| github.com/invopop/jsonschema | v0.14.0 | runtime binary; Go tests | COPYING |
| github.com/jackc/pgpassfile | v1.0.0 | runtime binary; Go tests | LICENSE |
| github.com/jackc/pgservicefile | v0.0.0-20240606120523-5a60cdf6a761 | runtime binary; Go tests | LICENSE |
| github.com/jackc/pgx/v5 | v5.11.0 | runtime binary; Go tests | LICENSE |
| github.com/jackc/puddle/v2 | v2.2.2 | runtime binary; Go tests | LICENSE |
| github.com/jordanlewis/gcassert | v0.0.0-20250430164644-389ef753e22e | module graph/build download only | LICENSE.txt |
| github.com/json-iterator/go | v1.1.12 | module graph/build download only | LICENSE |
| github.com/klauspost/compress | v1.17.6 | module graph/build download only | LICENSE |
| github.com/klauspost/cpuid/v2 | v2.3.0 | module graph/build download only | LICENSE |
| github.com/kr/pretty | v0.3.1 | module graph/build download only | License |
| github.com/kr/text | v0.2.0 | module graph/build download only | License |
| github.com/leodido/go-urn | v1.4.0 | runtime binary; Go tests | LICENSE |
| github.com/mattn/go-isatty | v0.0.20 | runtime binary; Go tests | LICENSE |
| github.com/modelcontextprotocol/go-sdk | v1.3.1 | module graph/build download only | LICENSE |
| github.com/modern-go/concurrent | v0.0.0-20180306012644-bacd9c7ef1dd | module graph/build download only | LICENSE |
| github.com/modern-go/reflect2 | v1.0.2 | module graph/build download only | LICENSE |
| github.com/pb33f/ordered-map/v2 | v2.3.1 | runtime binary; Go tests | LICENSE |
| github.com/pelletier/go-toml/v2 | v2.2.4 | runtime binary; Go tests | LICENSE |
| github.com/planetscale/vtprotobuf | v0.6.1-0.20240319094008-0393e58bdf10 | module graph/build download only | LICENSE |
| github.com/pmezard/go-difflib | v1.0.0 | module graph/build download only | LICENSE |
| github.com/prometheus/client_model | v0.0.0-20190812154241-14fe0d1b01d4 | module graph/build download only | LICENSE, NOTICE |
| github.com/quic-go/qpack | v0.6.0 | runtime binary; Go tests | LICENSE.md |
| github.com/quic-go/quic-go | v0.59.0 | runtime binary; Go tests | LICENSE |
| github.com/rogpeppe/go-internal | v1.10.0 | module graph/build download only | LICENSE |
| github.com/santhosh-tekuri/jsonschema/v6 | v6.0.3 | runtime binary; Go tests | LICENSE |
| github.com/segmentio/asm | v1.1.3 | module graph/build download only | LICENSE |
| github.com/segmentio/encoding | v0.5.4 | module graph/build download only | LICENSE |
| github.com/standard-webhooks/standard-webhooks/libraries | v0.0.1 | runtime binary; Go tests | LICENSE |
| github.com/stretchr/objx | v0.5.2 | module graph/build download only | LICENSE |
| github.com/stretchr/testify | v1.11.1 | module graph/build download only | LICENSE |
| github.com/tidwall/gjson | v1.18.0 | runtime binary; Go tests | LICENSE |
| github.com/tidwall/match | v1.1.1 | runtime binary; Go tests | LICENSE |
| github.com/tidwall/pretty | v1.2.1 | runtime binary; Go tests | LICENSE |
| github.com/tidwall/sjson | v1.2.5 | runtime binary; Go tests | LICENSE |
| github.com/twitchyliquid64/golang-asm | v0.15.1 | module graph/build download only | LICENSE |
| github.com/ugorji/go/codec | v1.3.1 | runtime binary; Go tests | LICENSE |
| github.com/xdg-go/pbkdf2 | v1.0.0 | module graph/build download only | LICENSE |
| github.com/xdg-go/scram | v1.2.0 | module graph/build download only | LICENSE |
| github.com/xdg-go/stringprep | v1.0.4 | module graph/build download only | LICENSE |
| github.com/yosida95/uritemplate/v3 | v3.0.2 | module graph/build download only | LICENSE |
| github.com/youmark/pkcs8 | v0.0.0-20240726163527-a2c0da244d78 | module graph/build download only | LICENSE |
| go.mongodb.org/mongo-driver/v2 | v2.5.0 | runtime binary; Go tests | LICENSE |
| go.opencensus.io | v0.24.0 | runtime binary; Go tests | LICENSE |
| go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc | v0.54.0 | module graph/build download only | LICENSE |
| go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp | v0.54.0 | module graph/build download only | LICENSE |
| go.opentelemetry.io/otel | v1.29.0 | module graph/build download only | LICENSE |
| go.opentelemetry.io/otel/metric | v1.29.0 | module graph/build download only | LICENSE |
| go.opentelemetry.io/otel/sdk | v1.29.0 | module graph/build download only | LICENSE |
| go.opentelemetry.io/otel/trace | v1.29.0 | module graph/build download only | LICENSE |
| go.uber.org/mock | v0.6.0 | module graph/build download only | LICENSE |
| go.yaml.in/yaml/v4 | v4.0.0-rc.2 | runtime binary; Go tests | LICENSE, NOTICE |
| golang.org/x/arch | v0.22.0 | module graph/build download only | LICENSE |
| golang.org/x/crypto | v0.48.0 | runtime binary; Go tests | LICENSE |
| golang.org/x/exp | v0.0.0-20190121172915-509febef88a4 | module graph/build download only | LICENSE |
| golang.org/x/lint | v0.0.0-20190313153728-d0100b6bd8b3 | module graph/build download only | LICENSE |
| golang.org/x/mod | v0.32.0 | module graph/build download only | LICENSE |
| golang.org/x/net | v0.51.0 | runtime binary; Go tests | LICENSE |
| golang.org/x/oauth2 | v0.37.0 | runtime binary; Go tests | LICENSE |
| golang.org/x/sync | v0.19.0 | runtime binary; Go tests | LICENSE |
| golang.org/x/sys | v0.41.0 | runtime binary; Go tests | LICENSE |
| golang.org/x/term | v0.40.0 | module graph/build download only | LICENSE |
| golang.org/x/text | v0.34.0 | runtime binary; Go tests | LICENSE |
| golang.org/x/time | v0.6.0 | module graph/build download only | LICENSE |
| golang.org/x/tools | v0.41.0 | module graph/build download only | LICENSE |
| golang.org/x/xerrors | v0.0.0-20191204190536-9bdfabe68543 | module graph/build download only | LICENSE |
| google.golang.org/api | v0.197.0 | module graph/build download only | LICENSE |
| google.golang.org/appengine | v1.6.8 | module graph/build download only | LICENSE |
| google.golang.org/genai | v1.71.0 | runtime binary; Go tests | LICENSE |
| google.golang.org/genproto | v0.0.0-20240903143218-8af14fe29dc1 | module graph/build download only | LICENSE |
| google.golang.org/genproto/googleapis/api | v0.0.0-20240903143218-8af14fe29dc1 | module graph/build download only | LICENSE |
| google.golang.org/genproto/googleapis/rpc | v0.0.0-20240903143218-8af14fe29dc1 | runtime binary; Go tests | LICENSE |
| google.golang.org/grpc | v1.66.2 | runtime binary; Go tests | LICENSE, NOTICE.txt |
| google.golang.org/protobuf | v1.36.10 | runtime binary; Go tests | LICENSE |
| gopkg.in/check.v1 | v1.0.0-20201130134442-10cb98267c6c | module graph/build download only | LICENSE |
| gopkg.in/yaml.v2 | v2.2.8 | module graph/build download only | LICENSE, LICENSE.libyaml, NOTICE |
| gopkg.in/yaml.v3 | v3.0.1 | module graph/build download only | LICENSE, NOTICE |
| honnef.co/go/tools | v0.0.0-20190523083050-ea95bdfd59fc | module graph/build download only | LICENSE |
| rsc.io/pdf | v0.1.1 | module graph/build download only | LICENSE |
| @axe-core/playwright | 4.13.0 | frontend build/test only | MPL-2.0; LICENSE |
| @babel/code-frame | 7.29.7 | frontend build/test only | MIT; LICENSE |
| @babel/helper-validator-identifier | 7.29.7 | frontend build/test only | MIT; LICENSE |
| @oxc-project/types | 0.150.0 | frontend build/test only | MIT; LICENSE |
| @playwright/test | 1.63.0 | frontend build/test only | Apache-2.0; LICENSE, NOTICE |
| @redocly/ajv | 8.11.2 | frontend build/test only | MIT; LICENSE |
| @redocly/config | 0.22.0 | frontend build/test only | MIT; LICENSE |
| @redocly/openapi-core | 1.34.20 | frontend build/test only | MIT; no top-level licence/notice file |
| @rolldown/binding-android-arm-eabi | 1.2.9 | frontend build/test only | MIT; no top-level licence/notice file |
| @rolldown/binding-android-arm64 | 1.2.9 | frontend build/test only | MIT; no top-level licence/notice file |
| @rolldown/binding-darwin-arm64 | 1.2.9 | frontend build/test only | MIT; no top-level licence/notice file |
| @rolldown/binding-darwin-x64 | 1.2.9 | frontend build/test only | MIT; no top-level licence/notice file |
| @rolldown/binding-freebsd-x64 | 1.2.9 | frontend build/test only | MIT; no top-level licence/notice file |
| @rolldown/binding-linux-arm-gnueabihf | 1.2.9 | frontend build/test only | MIT; no top-level licence/notice file |
| @rolldown/binding-linux-arm64-gnu | 1.2.9 | frontend build/test only | MIT; no top-level licence/notice file |
| @rolldown/binding-linux-arm64-musl | 1.2.9 | frontend build/test only | MIT; no top-level licence/notice file |
| @rolldown/binding-linux-ppc64-gnu | 1.2.9 | frontend build/test only | MIT; no top-level licence/notice file |
| @rolldown/binding-linux-s390x-gnu | 1.2.9 | frontend build/test only | MIT; no top-level licence/notice file |
| @rolldown/binding-linux-x64-gnu | 1.2.9 | frontend build/test only | MIT; LICENSE via rolldown@1.2.9 (matching repository, directory, version, and declared licence) |
| @rolldown/binding-linux-x64-musl | 1.2.9 | frontend build/test only | MIT; no top-level licence/notice file |
| @rolldown/binding-openharmony-arm64 | 1.2.9 | frontend build/test only | MIT; no top-level licence/notice file |
| @rolldown/binding-win32-arm64-msvc | 1.2.9 | frontend build/test only | MIT; no top-level licence/notice file |
| @rolldown/binding-win32-x64-msvc | 1.2.9 | frontend build/test only | MIT; no top-level licence/notice file |
| @rolldown/pluginutils | 1.0.1 | frontend build/test only | MIT; LICENSE |
| @tanstack/history | 1.162.4 | frontend bundle | MIT; LICENSE |
| @tanstack/query-core | 5.103.1 | frontend bundle | MIT; LICENSE |
| @tanstack/react-query | 5.103.1 | frontend bundle | MIT; LICENSE |
| @tanstack/react-router | 1.170.38 | frontend bundle | MIT; LICENSE |
| @tanstack/react-store | 0.11.1 | frontend bundle | MIT; LICENSE |
| @tanstack/router-core | 1.171.32 | frontend bundle | MIT; LICENSE |
| @tanstack/store | 0.11.1 | frontend bundle | MIT; LICENSE |
| @types/node | 26.6.2 | frontend build/test only | MIT; LICENSE |
| @types/react | 19.3.0 | frontend build/test only | MIT; LICENSE |
| @types/react-dom | 19.3.0 | frontend build/test only | MIT; LICENSE |
| @vitejs/plugin-react | 6.1.1 | frontend build/test only | MIT; LICENSE |
| agent-base | 7.1.4 | frontend build/test only | MIT; LICENSE |
| ansi-colors | 4.1.3 | frontend build/test only | MIT; LICENSE |
| argparse | 2.0.1 | frontend build/test only | Python-2.0; LICENSE |
| axe-core | 4.13.0 | frontend build/test only | MPL-2.0; LICENSE, LICENSE-3RD-PARTY.txt |
| balanced-match | 1.0.2 | frontend build/test only | MIT; LICENSE.md |
| brace-expansion | 2.1.7 | frontend build/test only | MIT; LICENSE |
| change-case | 5.4.4 | frontend build/test only | MIT; no top-level licence/notice file |
| colorette | 1.4.0 | frontend build/test only | MIT; LICENSE.md |
| cookie-es | 3.1.1 | frontend bundle | MIT; LICENSE |
| csstype | 3.2.3 | frontend build/test only | MIT; LICENSE |
| debug | 4.4.3 | frontend build/test only | MIT; LICENSE |
| detect-libc | 2.1.2 | frontend build/test only | Apache-2.0; LICENSE |
| fast-deep-equal | 3.1.3 | frontend build/test only | MIT; LICENSE |
| fdir | 6.5.0 | frontend build/test only | MIT; LICENSE |
| fsevents | 2.3.3 | frontend build/test only | MIT; no top-level licence/notice file |
| https-proxy-agent | 7.0.6 | frontend build/test only | MIT; LICENSE |
| index-to-position | 1.2.0 | frontend build/test only | MIT; license |
| isbot | 5.2.2 | frontend bundle | Unlicense; LICENSE |
| js-levenshtein | 1.1.6 | frontend build/test only | MIT; LICENSE |
| js-tokens | 4.0.0 | frontend build/test only | MIT; LICENSE |
| js-yaml | 4.3.2 | frontend build/test only | MIT; LICENSE |
| json-schema-traverse | 1.0.0 | frontend build/test only | MIT; LICENSE |
| lightningcss | 1.33.0 | frontend build/test only | MPL-2.0; LICENSE |
| lightningcss-android-arm64 | 1.33.0 | frontend build/test only | MPL-2.0; no top-level licence/notice file |
| lightningcss-darwin-arm64 | 1.33.0 | frontend build/test only | MPL-2.0; no top-level licence/notice file |
| lightningcss-darwin-x64 | 1.33.0 | frontend build/test only | MPL-2.0; no top-level licence/notice file |
| lightningcss-freebsd-x64 | 1.33.0 | frontend build/test only | MPL-2.0; no top-level licence/notice file |
| lightningcss-linux-arm-gnueabihf | 1.33.0 | frontend build/test only | MPL-2.0; no top-level licence/notice file |
| lightningcss-linux-arm64-gnu | 1.33.0 | frontend build/test only | MPL-2.0; no top-level licence/notice file |
| lightningcss-linux-arm64-musl | 1.33.0 | frontend build/test only | MPL-2.0; no top-level licence/notice file |
| lightningcss-linux-x64-gnu | 1.33.0 | frontend build/test only | MPL-2.0; LICENSE |
| lightningcss-linux-x64-musl | 1.33.0 | frontend build/test only | MPL-2.0; no top-level licence/notice file |
| lightningcss-win32-arm64-msvc | 1.33.0 | frontend build/test only | MPL-2.0; no top-level licence/notice file |
| lightningcss-win32-x64-msvc | 1.33.0 | frontend build/test only | MPL-2.0; no top-level licence/notice file |
| minimatch | 5.1.9 | frontend build/test only | ISC; LICENSE |
| ms | 2.1.3 | frontend build/test only | MIT; license.md |
| nanoid | 3.3.19 | frontend build/test only | MIT; LICENSE |
| openapi-typescript | 7.13.0 | frontend build/test only | MIT; LICENSE |
| parse-json | 8.3.0 | frontend build/test only | MIT; license |
| picocolors | 1.1.1 | frontend build/test only | ISC; LICENSE |
| picomatch | 4.0.7 | frontend build/test only | MIT; LICENSE |
| playwright | 1.63.0 | frontend build/test only | Apache-2.0; LICENSE, NOTICE |
| playwright-core | 1.63.0 | frontend build/test only | Apache-2.0; LICENSE, NOTICE |
| pluralize | 8.0.0 | frontend build/test only | MIT; LICENSE |
| postcss | 8.5.28 | frontend build/test only | MIT; LICENSE |
| react | 19.3.0 | frontend bundle | MIT; LICENSE |
| react-dom | 19.3.0 | frontend bundle | MIT; LICENSE |
| require-from-string | 2.0.2 | frontend build/test only | MIT; license |
| rolldown | 1.2.9 | frontend build/test only | MIT; LICENSE |
| scheduler | 0.28.0 | frontend bundle | MIT; LICENSE |
| seroval | 1.6.7 | frontend bundle | MIT; LICENSE |
| seroval-plugins | 1.6.7 | frontend bundle | MIT; LICENSE |
| source-map-js | 1.2.1 | frontend build/test only | BSD-3-Clause; LICENSE |
| supports-color | 10.2.2 | frontend build/test only | MIT; license |
| tinyglobby | 0.2.17 | frontend build/test only | MIT; LICENSE |
| type-fest | 4.41.0 | frontend build/test only | (MIT OR CC0-1.0); license-cc0, license-mit |
| typescript | 5.9.3 | frontend build/test only | Apache-2.0; LICENSE.txt |
| undici-types | 8.9.0 | frontend build/test only | MIT; LICENSE |
| uri-js-replace | 1.0.1 | frontend build/test only | MIT; no top-level licence/notice file |
| use-sync-external-store | 1.7.0 | frontend bundle | MIT; LICENSE |
| vite | 8.3.0 | frontend build/test only | MIT; LICENSE.md |
| yaml-ast-parser | 0.0.43 | frontend build/test only | Apache-2.0; license.txt |
| yargs-parser | 21.1.1 | frontend build/test only | ISC; LICENSE.txt |
| addyosmani/agent-skills | bcab6a1b8503100e8618c3b4e32cc78de43de769 | embedded agent skills | MIT; internal/skills/vendor/addyosmani/LICENSE |
| juliusbrussee/caveman | 2fd153c67988e980fb0b2455c90832159a6a5a25 | embedded agent skills | MIT; internal/skills/vendor/caveman/LICENSE |
