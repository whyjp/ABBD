use std::collections::HashMap;

pub fn run() -> usize {
    let mut m: HashMap<&str, usize> = HashMap::new();
    m.insert(lib::name(), 1);
    std::mem::size_of::<u8>() + m.len()
}
