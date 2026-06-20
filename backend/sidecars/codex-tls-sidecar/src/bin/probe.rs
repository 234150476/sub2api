#[tokio::main(flavor = "current_thread")]
async fn main() {
    rustls::crypto::aws_lc_rs::default_provider()
        .install_default()
        .expect("install rustls provider");

    println!("=== rustls aws_lc_rs DEFAULT_KX_GROUPS ===");
    let provider = rustls::crypto::CryptoProvider::get_default().expect("provider");
    for kx in provider.kx_groups.iter() {
        println!("  {:?}", kx.name());
    }

    let client = reqwest::Client::builder().build().expect("build client");

    let url = std::env::args()
        .nth(1)
        .unwrap_or_else(|| "https://tls.peet.ws/api/all".to_string());

    println!("=== GET {} ===", url);
    let body = client
        .get(&url)
        .send()
        .await
        .expect("send")
        .text()
        .await
        .expect("read body");
    println!("{}", body);
}
