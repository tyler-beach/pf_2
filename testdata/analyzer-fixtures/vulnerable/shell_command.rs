use std::process::Command;
fn main() {
    let user_input = std::env::var("USER_INPUT").unwrap_or_default();
    let _ = Command::new("sh").arg("-c").arg(user_input).status();
}
