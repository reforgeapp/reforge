import os
from urllib.parse import urlsplit, unquote, parse_qsl

keys = ['REFORGE_TEST_DATABASE_URL', 'REFORGE_TEST_MIGRATION_DATABASE_URL']
if os.environ.get('REFORGE_TEST_ADMIN_DATABASE_URL'):
    keys.append('REFORGE_TEST_ADMIN_DATABASE_URL')
urls = [urlsplit(os.environ[key]) for key in keys]
identities = []
for url in urls:
    if url.scheme not in ('postgres', 'postgresql') or unquote(url.path) != '/reforge_test':
        raise SystemExit('Integration URLs must explicitly name disposable database reforge_test')
    query = dict(parse_qsl(url.query))
    if set(query) - {'sslmode', 'host', 'port'}:
        raise SystemExit('Integration URLs allow only host, port and sslmode parameters')
    identities.append((query.get('host', url.hostname), str(query.get('port', url.port or 5432)), unquote(url.path)))
if any(identity != identities[0] for identity in identities[1:]):
    raise SystemExit('Runtime and migration URLs must identify the same disposable database')
