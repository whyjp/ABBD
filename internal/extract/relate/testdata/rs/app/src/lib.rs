use shapes::{Square, MAX};

mod net;
#[cfg(windows)]
mod win;

/// Runs [`engine::run`] once; the doc link is no call.
pub fn main_loop() -> String {
    let dog = plugin::Dog;
    let sq = Square { n: MAX };
    let _ = sq.n;
    engine::run(&dog)
}

fn helper() -> u32 {
    7
}

#[cfg(feature = "fancy")]
pub fn shine() -> u32 {
    fancy::glow()
}

#[cfg(test)]
mod tests {
    use extra::probe;

    #[test]
    fn probes() {
        assert_eq!(probe(), 1);
    }
}
