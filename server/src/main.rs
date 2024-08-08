mod helper;
use std::path::PathBuf;
use std::sync::Arc;
use axum::extract::ws::Message;
use futures_util::{SinkExt as _, StreamExt};
use axum::body::Body;
use axum::extract::{State, WebSocketUpgrade, ws::WebSocket};
use axum::http::Response;
use axum::response::IntoResponse;
use axum::routing::get;
use axum::Router;
use clap::Parser as _;
use helper::{ConfigArgs, Point, StadeData};
use image::RgbImage;
use image::ImageReader;
use lazy_static::lazy_static;
use tokio::sync::Mutex;

lazy_static! {
    static ref Config: ConfigArgs = ConfigArgs::parse();    
}

#[tokio::main]
async fn main() {
    println!("loading image...");
    let img_w = Config.width.unwrap_or(1000);
    let img_h = Config.height.unwrap_or(1000);
    let image = Arc::new(Mutex::new(RgbImage::new(img_w, img_h)));
    for x in 0..img_w {
        for y in 0..img_h {
            image.lock().await.put_pixel(x, y, image::Rgb([255, 255, 255]));
        }
    }
    let img_path = Config.save_location.clone().unwrap_or(PathBuf::from("place.png"));
    if !img_path.to_str().unwrap().ends_with(".png") {
        panic!("image path must end with .png");
    }
    if img_path.exists() {
        let result = load_old_image(img_path.clone(), image.clone()).await;
        if result.is_err() {
            println!("failed to load old image remove image to save new image");
            if img_path.is_file() {
                let _ = std::fs::remove_file(img_path.clone());
            } else {
                panic!("is not a file");
            }
        }
    }
    println!("completed loading image");
    let img_save_clone = image.clone();
    let img_path_cl = img_path.clone();
    tokio::spawn(async move {
        println!("start save image every {} seconds", Config.save_interval.unwrap_or(120));
        loop {
            tokio::time::sleep(std::time::Duration::from_secs(Config.save_interval.unwrap_or(120))).await;
            println!("scheduled start save image");
            let img_lock = img_save_clone.lock().await;
            let img = img_lock.to_owned();
            drop(img_lock);
            if Config.save_all_images.unwrap_or_default() {
                save_old_image().await;
            }
            let _ = img.save(img_path_cl.clone());
            let file_load_r = open_img(img_path_cl.clone()).await;
            if file_load_r.is_err() { 
                println!("failed to load image");
            }
            println!("scheduled save image completed");
        }
    });
    let (tx_set, mut rx_set) = tokio::sync::mpsc::unbounded_channel::<Point>();
    let (tx_noti, rx_noti) = async_channel::unbounded::<Point>();
    let image_cl = image.clone();
    tokio::spawn(async move {
        println!("start set pixel event listener");
        while let Some(ptr) = rx_set.recv().await {
            let img_lock = image_cl.lock().await;
            if ptr.x >= img_lock.width() || ptr.y >= img_lock.height() { continue; }
            drop(img_lock);
            image_cl.lock().await.put_pixel(ptr.x, ptr.y, ptr.color);
            let _ = tx_noti.send(ptr).await;
        }
    });
    let s_data = StadeData::new(image.clone(), tx_set, rx_noti);
    let app = Router::new()
                        .route("/ws", get(ws_hendler))
                        .route("/place.png", get(place_image))
                        .with_state(s_data);
    let address = Config.address.clone().unwrap_or("0.0.0.0:8080".to_string());
    let listener = tokio::net::TcpListener::bind(address.clone()).await.unwrap();
    println!("Start server at {}", address);
    let path_copy = img_path.clone();
    let _ = axum::serve(listener, app).with_graceful_shutdown(async move { 
        tokio::signal::ctrl_c().await.unwrap();
        println!("save image before exit");
        let img_lock = image.lock().await;
        let img = img_lock.to_owned();
        drop(img_lock);
        if Config.save_all_images.unwrap_or_default() {
            save_old_image().await;
        }
        let _ = img.save(path_copy);
        println!("save image completed");
        std::process::exit(0);
    }).await;
}

async fn save_old_image() {
    let namd_cl = Config.save_location.clone().unwrap_or(PathBuf::from("place.png")).clone();
    let mut new_old_file_name = namd_cl.to_str().unwrap().split(".").collect::<Vec<&str>>();
    let unix_time = std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap().as_secs().to_string();
    new_old_file_name.insert(new_old_file_name.len() - 2, unix_time.as_str());
    println!("{}", new_old_file_name.join("."));
    let _ = tokio::fs::rename(namd_cl.clone(), new_old_file_name.join(".")).await;
}

async fn open_img(path: PathBuf) -> Result<RgbImage, Box<dyn std::error::Error>> {
    let file_img_rs = ImageReader::open(&path)?;
    let img = file_img_rs.decode()?;
    let rgb_img = img.to_rgb8();
    Ok(rgb_img)
}

async fn load_old_image(old_image_path: PathBuf, image: Arc<Mutex<RgbImage>>) -> Result<(), Box<dyn std::error::Error>> {
    let image_lock = image.lock().await;
    let image_cl = image_lock.to_owned();
    drop(image_lock);
    let rgb_img = open_img(old_image_path).await?;
    let img_w = [image_cl.width(), rgb_img.width()].iter().min().unwrap().to_owned();
    let img_h = [image_cl.height(), rgb_img.height()].iter().min().unwrap().to_owned();
    for x in 0..img_w {
        for y in 0..img_h {
            let pixel = rgb_img.get_pixel(x, y);
            image.lock().await.put_pixel(x, y, *pixel);
        }
    }
    Ok(())
}



async fn ws_hendler(State(stade_data): State<StadeData>, ws: WebSocketUpgrade) -> impl IntoResponse {
    ws.on_upgrade(move |socket| handle_socket(socket, stade_data))
}

async fn handle_socket(socket: WebSocket, stade_data: StadeData) {

    let (mut sender, mut receiver) = socket.split();
    let notifyer = Arc::new(tokio::sync::Notify::new());

    let (tx_sender, mut rx_sender) = tokio::sync::mpsc::unbounded_channel::<Message>();
    let notifyer_cp = notifyer.clone();
    let ws_sender = tokio::spawn(async move {
        while let Some(msg) = rx_sender.recv().await {
            let _ = sender.send(msg).await;
        }
    });
    let tx_sender_cl = tx_sender.clone();
    let ws_receiver = tokio::spawn(async move {
        while let Some(Ok(msg)) = receiver.next().await {
            match msg.clone() {
                Message::Binary(data) => {
                    let _ = stade_data.sender.send(Point::from_byte(&data));
                },
                Message::Ping(data) => {
                    let _ = tx_sender_cl.send(Message::Pong(data));
                }
                Message::Pong(_) => {}
                Message::Close(_) => {
                    drop(tx_sender_cl);
                    notifyer_cp.notify_waiters();
                    break;
                }
                _ => {
                    drop(tx_sender_cl);
                    notifyer_cp.notify_waiters();
                    break;
                }
            }
        }
    });
    let tx_sender_cl = tx_sender.clone();
    let sync_data = tokio::spawn(async move {
        while let Ok(point) = stade_data.receiver.recv().await {
            let _ = tx_sender_cl.send(Message::Binary(point.to_byte()));
        }
    });
    let tx_sender_cl = tx_sender.clone();
    let send_ping = tokio::spawn(async move {
        loop {
            tokio::time::sleep(std::time::Duration::from_secs(30)).await;
            let r_send = tx_sender_cl.send(Message::Ping(vec![]));
            if r_send.is_err() { break; }
        }
    });
    notifyer.notified().await;
    ws_receiver.abort();
    ws_sender.abort();
    sync_data.abort();
    send_ping.abort();
    println!("Websocket context destroyed");
}

async fn place_image(State(stade_data): State<StadeData>) -> impl IntoResponse {
    let img_lock = stade_data.image.lock().await;
    let img = img_lock.to_owned();
    drop(img_lock);
    let mut crusor = std::io::Cursor::new(Vec::new());
    let _ = img.write_to(&mut crusor, image::ImageFormat::Png);
    let data = crusor.into_inner();
    Response::builder().header("content-type", "image/png").body(Body::from(data)).unwrap()
}
