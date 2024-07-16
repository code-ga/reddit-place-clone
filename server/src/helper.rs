use std::{path::PathBuf, sync::Arc};

use async_channel::Receiver;
use clap::Parser;
use image::RgbImage;
use tokio::sync::{mpsc::UnboundedSender, Mutex};

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
    pub color: image::Rgb<u8>
}

impl Point {
    pub fn new(x: u32, y: u32, r: u8, g: u8, b: u8) -> Self {
        let color = image::Rgb([r, g, b]);
        Self { x, y, color }
    }
    
    pub fn to_byte(&self) -> Vec<u8> {
        let mut result =  Vec::with_capacity(11);
        let x = self.x.to_be_bytes();
        result.extend(&x);
        let y = self.y.to_be_bytes();
        result.extend(&y);
        let r = self.color.0[0];
        let g = self.color.0[1];
        let b = self.color.0[2];
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
    pub image: Arc<Mutex<RgbImage>>,
    pub sender: UnboundedSender<Point>,
    pub receiver: Receiver<Point>
}

impl StadeData {
    pub fn new(image: Arc<Mutex<RgbImage>>, sender: UnboundedSender<Point>, receiver: Receiver<Point>) -> Self {
        Self { image, sender, receiver}
    }
}
