pub trait Speaker {
    fn speak(&self) -> String;
}

pub fn run(s: &dyn Speaker) -> String {
    s.speak()
}

#[macro_export]
macro_rules! loud {
    ($s:expr) => {
        $s.to_uppercase()
    };
}
