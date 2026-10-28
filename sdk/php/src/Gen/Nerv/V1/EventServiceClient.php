<?php
// GENERATED CODE -- DO NOT EDIT!

namespace Nerv\V1;

/**
 * EventService exposes nerv's hot-path event dispatch surface.
 * v0.1: Dispatch (synchronous handler fan-out) + List (event catalog).
 */
class EventServiceClient extends \Grpc\BaseStub {

    /**
     * @param string $hostname hostname
     * @param array $opts channel options
     * @param \Grpc\Channel $channel (optional) re-use channel object
     */
    public function __construct($hostname, $opts, $channel = null) {
        parent::__construct($hostname, $opts, $channel);
    }

    /**
     * Dispatch runs the matched hook handlers for an envelope and
     * returns the merged decision (block > rewrite > warn > allow).
     * @param \Nerv\V1\DispatchRequest $argument input argument
     * @param array $metadata metadata
     * @param array $options call options
     * @return \Grpc\UnaryCall<\Nerv\V1\DispatchResponse>
     */
    public function Dispatch(\Nerv\V1\DispatchRequest $argument,
      $metadata = [], $options = []) {
        return $this->_simpleRequest('/nerv.v1.EventService/Dispatch',
        $argument,
        ['\Nerv\V1\DispatchResponse', 'decode'],
        $metadata, $options);
    }

    /**
     * List returns the static catalog of known events from spec/events.yaml.
     * @param \Nerv\V1\ListRequest $argument input argument
     * @param array $metadata metadata
     * @param array $options call options
     * @return \Grpc\UnaryCall<\Nerv\V1\ListResponse>
     */
    public function List(\Nerv\V1\ListRequest $argument,
      $metadata = [], $options = []) {
        return $this->_simpleRequest('/nerv.v1.EventService/List',
        $argument,
        ['\Nerv\V1\ListResponse', 'decode'],
        $metadata, $options);
    }

}
