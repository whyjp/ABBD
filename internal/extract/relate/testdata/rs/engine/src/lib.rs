pub trait Speaker {
    fn speak(&self) -> String;
}

pub fn run(s: &dyn Speaker) -> String {
    s.speak()
}
