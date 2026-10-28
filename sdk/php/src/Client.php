<?php

declare(strict_types=1);

namespace HopTop\Nerv;

use Grpc\ChannelCredentials;
use Nerv\V1\Decision;
use Nerv\V1\Envelope;
use Nerv\V1\EventDescriptor;

/**
 * Client is the PHP-facing entry point for dispatching events to a
 * nerv daemon.
 *
 * Client delegates to an injectable {@see Transport}. Two transports
 * ship in-tree:
 *
 *   - {@see GrpcTransport} — real wire over PECL `grpc`. Construct via
 *     {@see Client::fromSocket()} (UDS) or {@see Client::fromBaseUrl()}
 *     (TCP). Requires the `grpc` PECL extension.
 *   - {@see EventServiceStub} — pure-PHP in-process Transport for unit
 *     tests + pre-wire dev wiring. Default when constructing
 *     `new Client()` without an explicit Transport.
 */
final class Client
{
    /**
     * Name of the environment variable consulted by
     * {@see self::defaultSocketPath()} to override the XDG-derived
     * default. Mirrors the Go SDK's `EnvSocket` constant.
     */
    public const ENV_SOCKET = 'NERV_SOCKET';

    private readonly Transport $transport;

    /**
     * @param Transport|null $transport Wire transport. Defaults to an
     *                                  in-process {@see EventServiceStub}.
     */
    public function __construct(?Transport $transport = null)
    {
        $this->transport = $transport ?? new EventServiceStub();
    }

    /**
     * Construct a Client bound to a nerv daemon's Unix socket path.
     *
     * Requires the PECL `grpc` extension. The constructor of the
     * underlying {@see GrpcTransport} throws a clear "install ext-grpc"
     * error if the extension is absent. For pure-PHP in-process
     * testing, use `new Client(new EventServiceStub())` instead.
     */
    public static function fromSocket(string $socketPath): self
    {
        // gRPC core requires the `unix:` URI scheme prefix to recognise
        // the target as a Unix domain socket rather than `host:port`.
        return new self(new GrpcTransport('unix:' . $socketPath));
    }

    /**
     * Construct a Client bound to a nerv daemon via a TCP URL.
     *
     * Accepts URLs of the form:
     *
     *   - `grpc://host:port` or `http://host:port` — insecure TCP
     *   - `grpcs://host:port` or `https://host:port` — TLS with system
     *     root certs (server-cert-only). For mTLS or custom CAs,
     *     construct `GrpcTransport` directly with a
     *     {@see ChannelCredentials::createSsl()} composed to taste and
     *     pass it to `new Client($transport)`.
     *   - bare `host:port` (no scheme) — insecure TCP, for parity with
     *     the gRPC core's target convention.
     *
     * Requires the PECL `grpc` extension; see {@see fromSocket()}.
     */
    public static function fromBaseUrl(string $baseUrl): self
    {
        $trimmed = trim($baseUrl);
        if ($trimmed === '') {
            throw new \InvalidArgumentException(
                'fromBaseUrl: empty URL',
            );
        }

        // Detect explicit schemes; otherwise assume bare host:port.
        if (str_contains($trimmed, '://')) {
            [$scheme, $rest] = explode('://', $trimmed, 2);
            $scheme = strtolower($scheme);
            $authority = $rest;
        } else {
            $scheme = 'grpc';
            $authority = $trimmed;
        }

        $tls = false;
        if ($scheme === 'grpcs' || $scheme === 'https') {
            $tls = true;
        } elseif ($scheme !== 'grpc' && $scheme !== 'http') {
            throw new \InvalidArgumentException(
                'fromBaseUrl: unsupported URL scheme: ' . $scheme,
            );
        }

        // Strip trailing path/query — gRPC targets are host:port only.
        $authority = strtok($authority, '/?#');
        if ($authority === false || $authority === '') {
            throw new \InvalidArgumentException(
                'fromBaseUrl: missing authority in URL: ' . $baseUrl,
            );
        }

        // ext-grpc is required to even reference ChannelCredentials.
        // Surface the install-instructions error from GrpcTransport's
        // canonical check rather than triggering a confusing
        // class-not-found error here.
        if (!class_exists(ChannelCredentials::class)) {
            // GrpcTransport's constructor throws with full instructions.
            return new self(new GrpcTransport($authority));
        }

        // TLS schemes use server-cert-only TLS with system roots.
        // createSsl(null, null, null) → system roots, no client cert.
        // mTLS / custom CA: construct GrpcTransport directly.
        $credentials = $tls
            ? ChannelCredentials::createSsl()
            : ChannelCredentials::createInsecure();

        return new self(new GrpcTransport($authority, $credentials));
    }

    /**
     * Return the conventional UDS path for a local nerv daemon.
     *
     * Resolution order (matches the Go SDK and the nerv daemon):
     *
     * 1. `$NERV_SOCKET` if set and non-empty
     * 2. `$XDG_RUNTIME_DIR/nerv.sock`
     * 3. `sys_get_temp_dir() . '/nerv.sock'` (fallback)
     */
    public static function defaultSocketPath(): string
    {
        $override = getenv(self::ENV_SOCKET);
        if (is_string($override) && $override !== '') {
            return $override;
        }
        $runtimeDir = getenv('XDG_RUNTIME_DIR');
        if (!is_string($runtimeDir) || $runtimeDir === '') {
            $runtimeDir = sys_get_temp_dir();
        }
        return rtrim($runtimeDir, DIRECTORY_SEPARATOR) . DIRECTORY_SEPARATOR . 'nerv.sock';
    }

    /** Dispatch a single envelope to the daemon. */
    public function dispatch(Envelope $envelope): Decision
    {
        return $this->transport->dispatch($envelope);
    }

    /**
     * List event descriptors known to the daemon.
     *
     * @return list<EventDescriptor>
     */
    public function list(): array
    {
        return $this->transport->list();
    }

    /** Release transport resources. Idempotent. */
    public function close(): void
    {
        $this->transport->close();
    }

    /** Underlying transport — exposed for tests / advanced wiring. */
    public function transport(): Transport
    {
        return $this->transport;
    }
}
