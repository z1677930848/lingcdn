fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Use vendored protoc to avoid system dependency.
    let protoc = protoc_bin_vendored::protoc_bin_path()?;
    std::env::set_var("PROTOC", protoc);

    tonic_build::configure()
        .build_server(false)
        .build_client(true)
        .compile(
            &[
                "../../proto/node_control.proto",
                "../../proto/admin_control.proto",
            ],
            &["../../proto"],
        )?;
    Ok(())
}
