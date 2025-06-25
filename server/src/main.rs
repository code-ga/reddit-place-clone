mod helper;

use std::path::PathBuf;
use tokio::sync::broadcast::error::RecvError;
use std::sync::Arc;

use axum::body::Body;
use axum::extract::ws::Message;
use axum::extract::{ws::WebSocket, State, WebSocketUpgrade};
use axum::http::Response;
use axum::response::IntoResponse;
use axum::routing::get;
use axum::Router;
use clap::Parser as _;
use futures_util::{SinkExt as _, StreamExt};
use image::{ImageReader, RgbImage};
use lazy_static::lazy_static;

use helper::{ConfigArgs, StadeData};

lazy_static! {
    static ref Config: ConfigArgs = ConfigArgs::parse();
}

#[tokio::main]
async fn main() {
    println!("loading image...");

    let img_w = Config.width.unwrap_or(1000);
    let img_h = Config.height.unwrap_or(1000);
    let channel_size = Config.channel_size.unwrap_or(1024);
    let s_data = StadeData::new(img_w, img_h, channel_size);

    let img_path = Config
        .save_location
        .clone()
        .unwrap_or(PathBuf::from("place.png"));
    if !img_path.to_str().unwrap().ends_with(".png") {
        panic!("image path must end with .png");
    } else if img_path.exists() {
        println!("loading old image from {}", img_path.to_str().unwrap());
        let img = open_img(img_path.clone()).await;
        match img {
            Ok(img) => {
                s_data.load_from_old_image(&img);
                println!("image loaded successfully");
            }
            Err(e) => {
                eprintln!("error loading image: {}", e);
                println!("image exists but could not be loaded, creating new one");
            }
        }
    } else {
        println!("image not found, created new one");
    }

    let save_interval = Config.save_interval.unwrap_or(120);
    let s_data_cp = s_data.clone();
    let img_path_cp = img_path.clone();
    tokio::spawn(async move {
        loop {
            tokio::time::sleep(std::time::Duration::from_secs(save_interval)).await;
            if Config.save_all_images.unwrap_or(false) {
                save_old_image().await;
            }
            let status = s_data_cp.get_image().save(
                img_path_cp.clone(),
            );
            match status {
                Ok(_) => println!("image saved successfully"),
                Err(e) => eprintln!("error saving image: {}", e),
            }
        }
    });

    let app = Router::new()
        .route("/ws", get(ws_hendler))
        .route("/place.png", get(place_image))
        .with_state(s_data.clone());

    let address = Config.address.clone().unwrap_or("0.0.0.0:8080".to_string());
    let listener = tokio::net::TcpListener::bind(address.clone())
        .await
        .unwrap();
    println!("Start server at {}", address);

    let path_copy = img_path.clone();
    let s_data_shutdown = s_data.clone();

    let _ = axum::serve(listener, app)
        .with_graceful_shutdown(async move {
            tokio::signal::ctrl_c().await.unwrap();
            println!("save image before exit");
            if Config.save_all_images.unwrap_or(false) {
                save_old_image().await;
            }
            let status = s_data_shutdown.get_image().save(path_copy);
            match status {
                Ok(_) => println!("image saved successfully before exit"),
                Err(e) => eprintln!("error saving image before exit: {}", e),
            }
        })
        .await;
}

async fn save_old_image() {
    let namd_cl = Config
        .save_location
        .clone()
        .unwrap_or(PathBuf::from("place.png"))
        .clone();
    let mut new_old_file_name = namd_cl.to_str().unwrap().split(".").collect::<Vec<&str>>();
    let unix_time = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_secs()
        .to_string();

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

async fn ws_hendler(
    State(stade_data): State<StadeData>,
    ws: WebSocketUpgrade,
) -> impl IntoResponse {
    ws.on_upgrade(move |socket| handle_socket(socket, stade_data))
}

async fn handle_socket(socket: WebSocket, stade_data: StadeData) {
    let (mut sender, mut receiver) = socket.split();
    let notifyer = Arc::new(tokio::sync::Notify::new());

    let (tx_sender, mut rx_sender) = tokio::sync::mpsc::unbounded_channel::<Message>();
    
    let notifyer_cp = notifyer.clone();
    let ws_sender = tokio::spawn(async move {
        let notifyer_cp2 = notifyer_cp.clone();
        while let Some(msg) = rx_sender.recv().await {
            match sender.send(msg).await {
                Ok(_) => {},
                Err(_) => notifyer_cp2.notified().await
            }
        }
    });

    let tx_sender_cl = tx_sender.clone();
    let stade_data_clone = stade_data.clone();
    let notifyer_cp = notifyer.clone();
    let ws_receiver = tokio::spawn(async move {
        while let Some(Ok(msg)) = receiver.next().await {
            match msg.clone() {
                Message::Binary(data) => {
                    let _ = stade_data_clone.set_pixel(&data).await;
                }
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
    let notifyer_cp = notifyer.clone();
    let sync_data = tokio::spawn(async move {
        let mut data_receiver = stade_data.listen();
        loop {
            match data_receiver.recv().await {
                Ok(point) => {
                    if tx_sender_cl.send(Message::Binary(point.to_byte())).is_err() {
                        notifyer_cp.notify_waiters();
                        break;
                    }
                },
                Err(RecvError::Lagged(num)) => {
                    eprintln!("Lagged {} messages, skipping", num);
                }
                _ => ()
            }
        }
    });

    let tx_sender_cl = tx_sender.clone();
    let send_ping = tokio::spawn(async move {
        loop {
            tokio::time::sleep(std::time::Duration::from_secs(10)).await;
            let r_send = tx_sender_cl.send(Message::Ping(vec![]));
            if r_send.is_err() {
                break;
            }
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
    let img = stade_data.get_image();
    let mut crusor = std::io::Cursor::new(Vec::new());
    let _ = img.write_to(&mut crusor, image::ImageFormat::Png);
    let data = crusor.into_inner();

    Response::builder()
        .header("content-type", "image/png")
        .header("Access-Control-Allow-Origin", "*")
        .header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        .body(Body::from(data))
        .unwrap()
}
