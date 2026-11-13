use engine::Speaker;

pub struct Dog;

impl Speaker for Dog {
    fn speak(&self) -> String {
        engine::loud!("woof")
    }
}
