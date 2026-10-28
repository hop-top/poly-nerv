//! Integration tests for `hop-top-nerv`.
//!
//! Exercises the public crate surface. The UDS round-trip tests spin
//! up an in-process tonic server bound to a temp socket — the same
//! shape the Go SDK uses in `sdk/go/client_test.go`.

use std::path::PathBuf;
use std::time::Duration;

use async_trait::async_trait;
use hop_top_nerv::Client;
use hop_top_nerv::nerv_v1::event_service_server::{EventService, EventServiceServer};
use hop_top_nerv::nerv_v1::{
    Action, Decision, DispatchRequest, DispatchResponse, Envelope, EventDescriptor, ListRequest,
    ListResponse,
};
use tempfile::TempDir;
use tokio::net::UnixListener;
use tokio::sync::oneshot;
use tokio_stream::wrappers::UnixListenerStream;
use tonic::transport::Server;
use tonic::{Request, Response, Status};

/// In-process EventService that echoes a fixed decision and catalog.
#[derive(Default)]
struct FakeEventService;

#[async_trait]
impl EventService for FakeEventService {
    async fn dispatch(
        &self,
        request: Request<DispatchRequest>,
    ) -> Result<Response<DispatchResponse>, Status> {
        let envelope = request.into_inner().envelope.unwrap_or_default();
        let message = if envelope.event.is_empty() {
            "ok".to_string()
        } else {
            format!("received:{}", envelope.event)
        };
        Ok(Response::new(DispatchResponse {
            decision: Some(Decision {
                action: Action::Allow as i32,
                message,
                rewrite: None,
                metadata: None,
            }),
        }))
    }

    async fn list(&self, _request: Request<ListRequest>) -> Result<Response<ListResponse>, Status> {
        Ok(Response::new(ListResponse {
            events: vec![
                EventDescriptor {
                    name: "PreToolUse".into(),
                    category: "tool".into(),
                    origin: "native".into(),
                    blocking: true,
                    ..Default::default()
                },
                EventDescriptor {
                    name: "SessionStart".into(),
                    category: "session".into(),
                    origin: "native".into(),
                    ..Default::default()
                },
            ],
        }))
    }
}

/// Spin up the fake EventService on a fresh tmp UDS path. Returns the
/// socket path, a TempDir guard (must outlive the test), and a
/// oneshot sender to trigger server shutdown.
async fn start_fake_daemon() -> (PathBuf, TempDir, oneshot::Sender<()>) {
    let dir = tempfile::Builder::new()
        .prefix("hop-top-nerv-")
        .tempdir()
        .expect("create tempdir");
    let sock_path = dir.path().join("n.sock");

    let listener = UnixListener::bind(&sock_path).expect("bind UDS");
    let stream = UnixListenerStream::new(listener);
    let (tx, rx) = oneshot::channel::<()>();

    let svc = EventServiceServer::new(FakeEventService);
    tokio::spawn(async move {
        let _ = Server::builder()
            .add_service(svc)
            .serve_with_incoming_shutdown(stream, async {
                let _ = rx.await;
            })
            .await;
    });

    // Wait briefly for the listener to be ready to accept.
    for _ in 0..50 {
        if sock_path.exists() {
            break;
        }
        tokio::time::sleep(Duration::from_millis(20)).await;
    }

    (sock_path, dir, tx)
}

#[tokio::test]
async fn dispatch_over_uds_round_trip() {
    let (sock, _dir, shutdown) = start_fake_daemon().await;
    let mut client = Client::connect_uds(&sock).await.expect("connect");

    let decision = client
        .dispatch(Envelope {
            nerv_version: "1.0.0-draft".into(),
            event: "PreToolUse".into(),
            event_id: "00000000-0000-0000-0000-000000000000".into(),
            ..Default::default()
        })
        .await
        .expect("dispatch");

    assert_eq!(decision.action, Action::Allow as i32);
    assert_eq!(decision.message, "received:PreToolUse");

    let _ = shutdown.send(());
}

#[tokio::test]
async fn list_over_uds_round_trip() {
    let (sock, _dir, shutdown) = start_fake_daemon().await;
    let mut client = Client::connect_uds(&sock).await.expect("connect");

    let events = client.list().await.expect("list");
    assert_eq!(events.len(), 2);
    assert_eq!(events[0].name, "PreToolUse");
    assert_eq!(events[1].name, "SessionStart");

    let _ = shutdown.send(());
}

#[tokio::test]
async fn close_consumes_client_after_use() {
    let (sock, _dir, shutdown) = start_fake_daemon().await;
    let mut client = Client::connect_uds(&sock).await.expect("connect");
    let _ = client.list().await.expect("list");
    client.close();
    let _ = shutdown.send(());
}

#[tokio::test]
async fn close_safe_on_lazy_channel() {
    // Equivalent to the go SDK's TestClient_Close_Idempotent: close a
    // client that was never dialed. Construction succeeds via
    // connect_lazy because it never blocks on I/O.
    let channel = tonic::transport::Endpoint::try_from("http://127.0.0.1:1")
        .expect("endpoint parses")
        .connect_lazy();
    let client = Client::from_channel(channel);
    client.close();
}

#[tokio::test]
async fn lazy_channel_yields_client() {
    let channel = tonic::transport::Endpoint::try_from("http://127.0.0.1:1")
        .expect("endpoint parses")
        .connect_lazy();
    let _client = Client::from_channel(channel);
}

#[test]
fn decision_default_matches_proto_zero_value() {
    let d = Decision::default();
    assert_eq!(d.action, Action::Unspecified as i32);
    assert!(d.message.is_empty());
    assert!(d.rewrite.is_none());
    assert!(d.metadata.is_none());
}
