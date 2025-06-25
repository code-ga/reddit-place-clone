use std::{path::PathBuf, sync::Arc};
use clap::Parser;
use dashmap::DashMap;
use image::Rgb;
use tokio::sync::broadcast::{Receiver, Sender};

#[derive(Parser, Debug, Clone)]
pub struct ConfigArgs {
    #[arg(short, long)]
    pub address: Option<String>,
    #[arg(long)]
    pub height: Option<u32>,
    #[arg(long)]
    pub width: Option<u32>,
    #[arg(short='i', long)]
    pub save_interval: Option<u64>,
    #[arg(short='l', long)]
    pub save_location: Option<PathBuf>,
    #[arg(short, long)]
    pub save_all_images: Option<bool>
}

#[derive(Debug, Clone)]
pub struct Point {
    pub x: u32,
    pub y: u32,
    pub color: [u8; 3]
}

impl Point {
    pub fn new(x: u32, y: u32, r: u8, g: u8, b: u8) -> Self {
        let color = [r, g, b];
        Self { x, y, color }
    }
    
    pub fn to_byte(&self) -> Vec<u8> {
        let mut result =  Vec::with_capacity(11);
        let x = self.x.to_be_bytes();
        result.extend(&x);
        let y = self.y.to_be_bytes();
        result.extend(&y);
        let r = self.color[0];
        let g = self.color[1];
        let b = self.color[2];
        result.extend(&[r,g,b]);
        result
    }

    pub fn from_byte(data: &[u8]) -> Self {
        let x = u32::from_be_bytes(data[0..4].try_into().unwrap());
        let y = u32::from_be_bytes(data[4..8].try_into().unwrap());
        let r = data[8];
        let g = data[9];
        let b = data[10];
        Self::new(x, y, r, g, b)
    }
}

#[derive(Debug, Clone)]
pub struct StadeData {
    width: u32,
    height: u32,
    image_dashmap: Arc<DashMap<u32, [u8; 3]>>,
    global_receiver: Arc<Receiver<Point>>,
    global_notifier: Sender<Point>
}

impl StadeData {
    pub fn new(width: u32, height: u32) -> Self {
        let (global_notifier, global_receiver) = tokio::sync::broadcast::channel::<Point>(10);
        let image_dashmap = Arc::new(DashMap::new());
        Self { image_dashmap, global_receiver: Arc::new(global_receiver), global_notifier, width, height }
    }

    fn coordinates_to_index(&self, x: u32, y: u32) -> u32 {
        let width = self.image_dashmap.len() as u32;
        y * width + x
    }

    pub async fn set_pixel(&self, raw: &[u8]) {
        let point = Point::from_byte(raw);
        self.global_notifier.send(point.clone()).unwrap();
        if point.color == [255; 3] {
            self.image_dashmap.remove(&self.coordinates_to_index(point.x, point.y));
            return;
        }
        self.image_dashmap.insert(self.coordinates_to_index(point.x, point.y), point.color);
    }

    pub fn get_image(&self) -> image::RgbImage {
        let mut image_raw = vec![255u8; (self.width * self.height * 3) as usize];
        
        for entry in self.image_dashmap.iter() {
            let index = *entry.key();
            let x = index % self.width;
            let y = index / self.width;
            let pixel_index = ((y * self.width + x) * 3) as usize;
            let color = entry.value();
            
            image_raw[pixel_index] = color[0];
            image_raw[pixel_index + 1] = color[1];
            image_raw[pixel_index + 2] = color[2];
        }
        
        let image = image::RgbImage::from_raw(
            self.width,
            self.height,
            image_raw
        ).expect("Failed to create image buffer");
        image
    }

    pub fn listen(&self) -> Receiver<Point> {
        self.global_receiver.resubscribe()
    }

    pub fn load_from_old_image(&self, image: &image::RgbImage) {
        for (x, y, pixel) in image.enumerate_pixels() {
            if x >= self.width || y >= self.height {
                continue; // Skip pixels outside the bounds
            } else if *pixel == Rgb([255, 255, 255]) {
                continue; // Skip white pixels
            }
            self.image_dashmap.insert(
                self.coordinates_to_index(x, y),
                [pixel[0], pixel[1], pixel[2]]
            );
        }
    }

}
