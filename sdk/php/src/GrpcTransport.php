<?php

declare(strict_types=1);

namespace HopTop\Nerv;

use Grpc\BaseStub;
use Grpc\ChannelCredentials;
use Nerv\V1\Decision;
use Nerv\V1\DispatchRequest;
use Nerv\V1\Envelope;
use Nerv\V1\EventDescriptor;
use Nerv\V1\EventServiceClient;
use Nerv\V1\ListRequest;

/**
 * GrpcTransport is the real wire transport for the nerv PHP SDK.
 *
 * Wraps the generated {@see EventServiceClient} client stub, which
 * extends {@see BaseStub} from the `grpc/grpc` Composer package and
 * runs on the PECL `grpc` extension. Supports both Unix domain
 * socket targets (`unix:/path/to/sock`) and TCP host:port targets
 * (`host:port` or scheme-prefixed URIs via {@see Client::fromBaseUrl()}).
 *
 * The class is defined unconditionally so the SDK loads under PSR-4
 * autoload even when the PECL `grpc` extension is absent. The
 * constructor throws a clear "install ext-grpc" error when the
 * extension is not loaded — discoverable at the call site, not at
 * composer-install time.
 *
 * For pure-PHP in-process testing (no PECL dep), use
 * {@see EventServiceStub} instead.
 *
 * ## Credentials seam
 *
 * The constructor takes a single optional `$credentials` parameter
 * (defaults to {@see ChannelCredentials::createInsecure()}). Callers
 * choose the credentials object — insecure, server-cert TLS via
 * `ChannelCredentials::createSsl()`, or mTLS via `createSsl()` with a
 * client cert chain + private key (optionally composed with call
 * credentials via `createComposite()`).
 *
 * This bring-your-own-credentials shape mirrors the Go SDK's option
 * pattern (e.g. `WithHTTPClient`): one constructor, one knob,
 * caller-supplied object. The `Client::fromBaseUrl()` factory picks
 * `createInsecure()` for `grpc://`/`http://`/bare and `createSsl()`
 * for `grpcs://`/`https://`; for mTLS or custom roots, instantiate
 * `GrpcTransport` directly and pass `new Client($transport)`.
 */
final class GrpcTransport implements Transport
{
    private EventServiceClient $stub;

    /**
     * @param string $target gRPC target URI. Use `unix:/abs/path.sock`
     *                       for Unix domain sockets, `host:port` for
     *                       insecure TCP. The `unix:` URI prefix is
     *                       required for the gRPC core to recognise
     *                       the path as a UDS rather than `host:port`.
     * @param ChannelCredentials|null $credentials Channel credentials.
     *                       Defaults to insecure. Pass a TLS-configured
     *                       credentials object for `grpcs://` targets.
     *
     * @throws \RuntimeException if PECL `grpc` extension is not loaded.
     */
    public function __construct(
        string $target,
        ?ChannelCredentials $credentials = null,
    ) {
        // Check the extension itself: Grpc\BaseStub comes from the
        // grpc/grpc Composer package and exists with or without it.
        if (!extension_loaded('grpc')) {
            throw new \RuntimeException(
                'PECL grpc extension not loaded. Install via '
                . '`pecl install grpc` (or `apt install php-grpc` on '
                . 'Debian/Ubuntu). See sdk/php/README.md for details.',
            );
        }
        $this->stub = new EventServiceClient($target, [
            'credentials' => $credentials ?? ChannelCredentials::createInsecure(),
        ]);
    }

    public function dispatch(Envelope $envelope): Decision
    {
        $request = new DispatchRequest();
        $request->setEnvelope($envelope);

        /** @var \Grpc\UnaryCall $call */
        $call = $this->stub->Dispatch($request);
        [$response, $status] = $call->wait();

        $this->assertOk($status, 'Dispatch');

        $decision = $response?->getDecision();
        if ($decision === null) {
            throw new \RuntimeException(
                'nerv daemon returned DispatchResponse without a decision',
            );
        }
        return $decision;
    }

    /** @return list<EventDescriptor> */
    public function list(): array
    {
        $request = new ListRequest();

        /** @var \Grpc\UnaryCall $call */
        $call = $this->stub->List($request);
        [$response, $status] = $call->wait();

        $this->assertOk($status, 'List');

        $events = $response?->getEvents();
        if ($events === null) {
            return [];
        }
        // Normalise the RepeatedField to a list<EventDescriptor> so the
        // Transport contract's return type stays a plain PHP array.
        $out = [];
        foreach ($events as $event) {
            $out[] = $event;
        }
        return $out;
    }

    public function close(): void
    {
        // BaseStub channels are reclaimed by the PECL extension on
        // object destruction; explicit close is a no-op so the
        // Transport contract is satisfied.
    }

    /**
     * Map a gRPC status object to a RuntimeException if non-OK.
     *
     * The PECL grpc extension returns status as a stdClass with `code`
     * (int) and `details` (string) fields. `Grpc\STATUS_OK` is 0.
     *
     * @param object $status
     */
    private function assertOk(object $status, string $rpc): void
    {
        $code = $status->code ?? null;
        if ($code === 0) {
            return;
        }
        $details = $status->details ?? '';
        throw new \RuntimeException(
            sprintf(
                'nerv %s RPC failed: gRPC status code %s: %s',
                $rpc,
                var_export($code, true),
                $details,
            ),
        );
    }
}
