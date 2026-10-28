//! Transport builders: turn a socket path or TCP endpoint into a
//! [`tonic::transport::Channel`].

use std::path::PathBuf;
use std::sync::Arc;

use tonic::transport::{Channel, Endpoint, Uri};
use tower::service_fn;

use crate::error::ConnectError;

/// Build a `Channel` that speaks gRPC over a Unix domain socket.
///
/// The HTTP authority is a placeholder; tonic still requires a valid
/// URI shape for routing purposes but the connector ignores it.
pub(crate) async fn uds_channel(socket_path: PathBuf) -> Result<Channel, ConnectError> {
    let path: Arc<PathBuf> = Arc::new(socket_path);
    let endpoint = Endpoint::try_from("http://[::1]:50051")?;
    let channel = endpoint
        .connect_with_connector(service_fn(move |_: Uri| {
            let path = Arc::clone(&path);
            async move {
                let stream = tokio::net::UnixStream::connect(path.as_ref()).await?;
                Ok::<_, std::io::Error>(hyper_util::rt::TokioIo::new(stream))
            }
        }))
        .await?;
    Ok(channel)
}

/// Build a `Channel` that speaks gRPC over plain TCP (h2c).
///
/// Mostly useful for tests where a Unix socket is inconvenient.
pub(crate) async fn tcp_channel(endpoint: impl Into<String>) -> Result<Channel, ConnectError> {
    let channel = Endpoint::from_shared(endpoint.into())?.connect().await?;
    Ok(channel)
}
