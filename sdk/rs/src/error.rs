//! Error types surfaced by [`crate::Client`].

use thiserror::Error;

/// Error returned when a client cannot establish a transport channel.
#[derive(Debug, Error)]
pub enum ConnectError {
    /// Failed to construct or open the underlying transport.
    #[error("transport error: {0}")]
    Transport(#[from] tonic::transport::Error),
}

/// Error returned by RPC methods on [`crate::Client`].
#[derive(Debug, Error)]
pub enum DispatchError {
    /// gRPC call returned a non-OK status.
    #[error("grpc status: {0}")]
    Status(#[from] tonic::Status),

    /// Server returned a response without the expected payload.
    #[error("server response was missing required field: {0}")]
    MissingField(&'static str),
}
