// build.rs — compile nerv proto into Rust types via tonic-prost-build.
//
// buf.gen.yaml deliberately skips Rust codegen; tonic-prost-build is the
// idiomatic path for the Rust ecosystem and runs as part of `cargo
// build`, so consumers do not need buf installed.

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let proto_root = "../../contracts/proto";
    let proto_file = format!("{proto_root}/nerv/v1/event.proto");

    println!("cargo:rerun-if-changed={proto_file}");
    println!("cargo:rerun-if-changed=build.rs");

    // Emit BOTH client + server stubs. Runtime SDK consumers only see
    // the client; the server trait is needed by the integration tests
    // to spin up an in-process fake daemon over a tmp UDS path.
    tonic_prost_build::configure()
        .build_server(true)
        .build_client(true)
        .compile_protos(&[proto_file.as_str()], &[proto_root])?;

    Ok(())
}
