<?php

declare(strict_types=1);

namespace HopTop\Nerv;

/**
 * Transport is the wire-level contract between Client and a nerv daemon.
 *
 * In this alpha scaffold no real implementation ships: Connect-PHP is not
 * yet on Packagist and the PECL grpc extension is optional. The default
 * EventServiceStub satisfies the interface for unit tests and dev wiring;
 * a real Connect-over-UDS transport will land in a follow-up task once
 * Connect-PHP releases.
 *
 * Implementations MUST be safe to call from a single PHP request; they
 * are NOT required to be thread-safe.
 */
interface Transport
{
    /**
     * Dispatch a single envelope and return the daemon's decision.
     *
     * Implementations encode the envelope, send it to the daemon over
     * its configured wire (Unix socket, TCP), and decode the response.
     */
    public function dispatch(\Nerv\V1\Envelope $envelope): \Nerv\V1\Decision;

    /**
     * List event descriptors currently registered with the daemon.
     *
     * @return list<\Nerv\V1\EventDescriptor>
     */
    public function list(): array;

    /**
     * Release any underlying resources (sockets, channels).
     *
     * Idempotent: callers may invoke close() multiple times.
     */
    public function close(): void;
}
