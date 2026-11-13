#[cfg(any())]
pub fn never() -> u32 {
    util::wide()
}

#[cfg(windows)]
pub fn on_windows() -> String {
    engine::run(&plugin::Dog)
}

#[cfg(feature = "plain")]
pub fn plain() -> u32 {
    util::wide()
}

pub fn always() -> u32 {
    #[cfg(windows)]
    let _ = engine::loud!("x");
    shapes::MAX
}

#[cfg(test)]
mod tests {
    pub use plugin::Dog;
}
