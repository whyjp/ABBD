use engine::Speaker;

pub struct Dog;

impl Speaker for Dog {
    fn speak(&self) -> String {
        "woof".to_string()
    }
}
