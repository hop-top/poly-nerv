//! `Client` — thin wrapper around the tonic-generated
//! `EventServiceClient`.

use std::path::Path;

use tonic::transport::Channel;

use crate::error::{ConnectError, DispatchError};
use crate::nerv_v1::{
    Decision, DispatchRequest, Envelope, EventDescriptor, ListRequest,
    event_service_client::EventServiceClient,
};
use crate::transport;

/// Client for a running `nerv daemon`.
///
/// Construct one with [`Client::connect_uds`] (Unix socket) or
/// [`Client::connect_tcp`] (TCP h2c, intended for tests).
#[derive(Debug, Clone)]
pub struct Client {
    inner: EventServiceClient<Channel>,
}

impl Client {
    /// Build a client from an already-constructed gRPC channel.
    pub fn from_channel(channel: Channel) -> Self {
        Self {
            inner: EventServiceClient::new(channel),
        }
    }

    /// Connect to a `nerv daemon` listening on a Unix domain socket.
    pub async fn connect_uds(socket_path: impl AsRef<Path>) -> Result<Self, ConnectError> {
        let channel = transport::uds_channel(socket_path.as_ref().to_path_buf()).await?;
        Ok(Self::from_channel(channel))
    }

    /// Connect to a `nerv daemon` over plain TCP (h2c). Test convenience.
    pub async fn connect_tcp(endpoint: impl Into<String>) -> Result<Self, ConnectError> {
        let channel = transport::tcp_channel(endpoint).await?;
        Ok(Self::from_channel(channel))
    }

    /// Dispatch an envelope and return the merged handler decision.
    pub async fn dispatch(&mut self, envelope: Envelope) -> Result<Decision, DispatchError> {
        let response = self
            .inner
            .dispatch(DispatchRequest {
                envelope: Some(envelope),
            })
            .await?;
        response
            .into_inner()
            .decision
            .ok_or(DispatchError::MissingField("decision"))
    }

    /// Fetch the static event catalog from the daemon.
    pub async fn list(&mut self) -> Result<Vec<EventDescriptor>, DispatchError> {
        let response = self.inner.list(ListRequest {}).await?;
        Ok(response.into_inner().events)
    }

    /// Release the underlying transport channel.
    ///
    /// Mirrors the Go SDK's `(*Client).Close()` contract: idempotent
    /// and safe to call on a never-used `Client`. tonic's `Channel`
    /// also cleans up on `Drop`; this method exists for surface
    /// symmetry with the Go SDK and for callers that want an explicit
    /// "I'm done" signal before the value goes out of scope.
    pub fn close(self) {
        // Dropping `self` (and therefore the inner Channel) shuts
        // down idle connections. No work to do beyond letting the
        // value fall out of scope.
        drop(self);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Smoke test: a stub channel can be wrapped into a `Client`
    /// without hitting the network. Exercises the public surface.
    #[tokio::test]
    async fn client_constructs_from_channel() {
        // Build a channel that does not connect. `connect_lazy` returns
        // a `Channel` immediately; the first RPC would fail, but we
        // just want to prove the type plumbing compiles + links.
        let channel = tonic::transport::Endpoint::try_from("http://127.0.0.1:1")
            .expect("endpoint parses")
            .connect_lazy();
        let _client = Client::from_channel(channel);
    }
}
