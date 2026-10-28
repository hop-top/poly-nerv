# nerv PHP SDK

PHP SDK for nerv — Universal Hooks Protocol.

Status: **alpha**. Real gRPC wire transport ships via PECL `grpc`.

## Install

```bash
composer require hop-top/nerv
```

Requires PHP 8.2+.

The package installs without native extensions — `EventServiceStub` (the
in-process Transport for unit tests and dev wiring) is pure PHP. The
real wire transports (`Client::fromSocket` + `Client::fromBaseUrl`)
require the PECL `grpc` extension:

```bash
# PECL (any platform)
pecl install grpc

# Debian / Ubuntu
sudo apt install php-grpc

# Homebrew (macOS) — `grpc` formula ships both the runtime extension
# and the protoc plugin; enable the extension in your php.ini.
brew install grpc
```

The extension is declared in `composer.json` under `suggest:` (not
`require:`) so the SDK installs cleanly on stub-only environments.
The pure-PHP `grpc/grpc` package (the `Grpc\BaseStub` client base
class the generated stubs extend) is a regular dependency; it installs
without the extension.
Calling `Client::fromSocket()` / `fromBaseUrl()` without the extension
throws a `RuntimeException` with the install command in the message.

## Usage

### Real wire (PECL `grpc` required)

```php
<?php

require __DIR__ . '/vendor/autoload.php';

use HopTop\Nerv\Client;
use Nerv\V1\CLIInfo;
use Nerv\V1\Envelope;
use Nerv\V1\SessionInfo;

// UDS — preferred for local daemon.
$client = Client::fromSocket(Client::defaultSocketPath());

// nervd itself listens only on a Unix socket. The TCP factories are
// for a daemon exposed through a TCP/TLS proxy.
// $client = Client::fromBaseUrl('grpc://127.0.0.1:50051');

// TCP TLS (server-cert with system roots) — for a daemon behind a
// TLS-terminating proxy. See the "TLS" section below for mTLS.
// $client = Client::fromBaseUrl('grpcs://nerv.example.com:443');

$envelope = (new Envelope())
    ->setNervVersion('1.0.0-draft')
    ->setEvent('PreToolUse')
    ->setEventId('example-1')
    ->setCli((new CLIInfo())->setName('claude')->setVersion('0.0.0'))
    ->setSession((new SessionInfo())->setId('s-1')->setCwd('/tmp'));

$decision = $client->dispatch($envelope);
printf("action=%d message=%s\n", $decision->getAction(), $decision->getMessage());

$client->close();
```

### In-process stub (no PECL required)

```php
<?php

require __DIR__ . '/vendor/autoload.php';

use HopTop\Nerv\Client;
use HopTop\Nerv\EventServiceStub;
use Nerv\V1\Action;
use Nerv\V1\Decision;
use Nerv\V1\Envelope;

$allow = (new Decision())->setAction(Action::ACTION_ALLOW);
$client = new Client((new EventServiceStub())->withQueuedDecision($allow));

$envelope = (new Envelope())->setEvent('PreToolUse');
$decision = $client->dispatch($envelope);

printf("action=%d\n", $decision->getAction());

$client->close();
```

See [`examples/dispatch.php`](https://github.com/hop-top/poly-nerv/blob/main/sdk/php/examples/dispatch.php) +
[`examples/list_events.php`](https://github.com/hop-top/poly-nerv/blob/main/sdk/php/examples/list_events.php) for runnable
variants; [`examples/README.md`](https://github.com/hop-top/poly-nerv/blob/main/sdk/php/examples/README.md) indexes them.

## Socket discovery

`HopTop\Nerv\Client::defaultSocketPath()` resolves in this order,
matching the Go SDK:

1. `$NERV_SOCKET` if set
2. `$XDG_RUNTIME_DIR/nerv.sock`
3. `sys_get_temp_dir() . '/nerv.sock'` (fallback)

The env-var name is exposed as `Client::ENV_SOCKET`.

## API surface

| Symbol | Purpose |
|--------|---------|
| `HopTop\Nerv\Client` | Public entry point. Construct with an optional `Transport`; defaults to `EventServiceStub`. |
| `Client::dispatch(Envelope): Decision` | Send one envelope; receive merged decision. |
| `Client::list(): EventDescriptor[]` | Fetch the daemon's event catalog. |
| `Client::close(): void` | Release transport resources. Idempotent. |
| `Client::fromSocket(string)` | Factory: opens a Unix socket connection to the daemon via `GrpcTransport`. Requires PECL `grpc`. |
| `Client::fromBaseUrl(string)` | Factory: opens a TCP connection. Insecure for `grpc://`, `http://`, or bare `host:port`; TLS (server-cert with system roots) for `grpcs://` or `https://`. Requires PECL `grpc`. |
| `Client::defaultSocketPath(): string` | Resolve the conventional socket path. |
| `Client::ENV_SOCKET` | Name of the env var consulted by `defaultSocketPath` (`NERV_SOCKET`). |
| `HopTop\Nerv\Transport` | Interface every wire backend implements. |
| `HopTop\Nerv\GrpcTransport` | Real wire transport over PECL `grpc`. Constructor throws if extension absent. |
| `HopTop\Nerv\EventServiceStub` | In-process Transport for tests + dev. Supports queued decisions + canned descriptors. |
| `Nerv\V1\*` | Protobuf message + service client classes generated from `contracts/proto/nerv/v1/event.proto` under `src/Gen/`. Do not edit by hand. |

## Wire transport

`GrpcTransport` wraps the PECL `grpc` extension via the generated
`Nerv\V1\EventServiceClient` (extends `Grpc\BaseStub`). UDS targets
require the `unix:` URI scheme prefix (gRPC core convention); the
`Client::fromSocket()` factory adds the prefix automatically.

```php
// UDS
$client = Client::fromSocket('/run/nerv.sock');
// Internally: new GrpcTransport('unix:/run/nerv.sock')

// TCP (insecure)
$client = Client::fromBaseUrl('grpc://daemon.local:50051');
// Or: Client::fromBaseUrl('daemon.local:50051');

// TCP (TLS — server cert with system root certs)
$client = Client::fromBaseUrl('grpcs://nerv.example.com:443');
// Or: Client::fromBaseUrl('https://nerv.example.com:443');
```

## TLS

`Client::fromBaseUrl()` picks the credentials object based on scheme:

| Scheme | Credentials |
|--------|-------------|
| `grpc://`, `http://`, bare `host:port` | `ChannelCredentials::createInsecure()` |
| `grpcs://`, `https://` | `ChannelCredentials::createSsl()` — server cert verified against system root certs, no client cert |

This covers a daemon exposed via a TCP/TLS proxy with a
publicly-rooted cert; nervd itself listens only on a Unix socket.

For mTLS, custom CA roots, or a client certificate, construct
`GrpcTransport` directly with a credentials object composed to taste
and pass it to the `Client` constructor:

```php
use HopTop\Nerv\Client;
use HopTop\Nerv\GrpcTransport;
use Grpc\ChannelCredentials;

$roots = file_get_contents('/etc/nerv/ca.pem');
$key   = file_get_contents('/etc/nerv/client.key');
$chain = file_get_contents('/etc/nerv/client.crt');

$tls = ChannelCredentials::createSsl($roots, $key, $chain);
$client = new Client(new GrpcTransport('nerv.example.com:443', $tls));
```

See PECL `grpc`'s
[`ChannelCredentials`](https://github.com/grpc/grpc/blob/master/src/php/ext/grpc/channel_credentials.c)
reference for the full credential composition surface
(`createComposite` etc.).

## Development

```bash
cd sdk/php
composer install
vendor/bin/phpunit
php -l src/Client.php
```

`composer.lock` is gitignored — library packages publish unlocked.

## License

MIT
