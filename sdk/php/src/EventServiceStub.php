<?php

declare(strict_types=1);

namespace HopTop\Nerv;

use Nerv\V1\Action;
use Nerv\V1\Decision;
use Nerv\V1\Envelope;
use Nerv\V1\EventDescriptor;

/**
 * EventServiceStub is an in-process Transport that never touches the wire.
 *
 * It exists so the SDK is loadable + testable today, before Connect-PHP
 * ships on Packagist. Behaviour:
 *
 *   - dispatch() returns Action::ACTION_ALLOW unless a canned decision
 *     was queued via withQueuedDecision().
 *   - list() returns whatever descriptors were registered via
 *     withDescriptors() (default: empty list).
 *   - close() is a no-op.
 *
 * Replace with the real Connect/gRPC transport once available.
 */
final class EventServiceStub implements Transport
{
    /** @var list<Decision> */
    private array $queuedDecisions = [];

    /** @var list<EventDescriptor> */
    private array $descriptors = [];

    /** @var list<Envelope> */
    private array $dispatched = [];

    public function withQueuedDecision(Decision $decision): self
    {
        $this->queuedDecisions[] = $decision;
        return $this;
    }

    /**
     * @param list<EventDescriptor> $descriptors
     */
    public function withDescriptors(array $descriptors): self
    {
        $this->descriptors = $descriptors;
        return $this;
    }

    public function dispatch(Envelope $envelope): Decision
    {
        $this->dispatched[] = $envelope;
        if ($this->queuedDecisions !== []) {
            return array_shift($this->queuedDecisions);
        }
        $decision = new Decision();
        $decision->setAction(Action::ACTION_ALLOW);
        return $decision;
    }

    /** @return list<EventDescriptor> */
    public function list(): array
    {
        return $this->descriptors;
    }

    public function close(): void
    {
        // no-op
    }

    /** @return list<Envelope> envelopes captured during dispatch(), in order */
    public function dispatchedEnvelopes(): array
    {
        return $this->dispatched;
    }
}
