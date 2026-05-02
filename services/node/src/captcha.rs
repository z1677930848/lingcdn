//! WAF Captcha generation module
//! Supports: Slide, Click, Rotate, SlideRegion, JsChallenge

use base64::{engine::general_purpose::STANDARD as BASE64, Engine};
use image::{Rgba, RgbaImage};
use rand::{thread_rng, Rng};
use serde::{Deserialize, Serialize};
use std::io::Cursor;

/// Captcha types
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum CaptchaType {
    Slide,
    Click,
    Rotate,
    SlideRegion,
    JsChallenge,
}

/// Slide captcha data for frontend
#[derive(Debug, Clone, Serialize)]
pub struct SlideData {
    pub image: String,      // base64 background
    pub thumb: String,      // base64 slider piece
    #[serde(rename = "thumbX")]
    pub thumb_x: u32,
    #[serde(rename = "thumbY")]
    pub thumb_y: u32,
    #[serde(rename = "thumbWidth")]
    pub thumb_width: u32,
    #[serde(rename = "thumbHeight")]
    pub thumb_height: u32,
}

/// Click captcha data
#[derive(Debug, Clone, Serialize)]
pub struct ClickData {
    pub image: String,  // base64 main image
    pub thumb: String,  // base64 hint image
}

/// Rotate captcha data
#[derive(Debug, Clone, Serialize)]
pub struct RotateData {
    pub image: String,
    pub thumb: String,
    pub angle: u32,
    #[serde(rename = "thumbSize")]
    pub thumb_size: u32,
}

/// Captcha answer for verification
#[derive(Debug, Clone)]
pub struct CaptchaAnswer {
    #[allow(dead_code)]
    pub captcha_type: CaptchaType,
    pub slide_x: Option<u32>,
    pub click_dots: Option<Vec<(u32, u32)>>,
    pub rotate_angle: Option<u32>,
}

/// Generate slide captcha
pub fn generate_slide_captcha() -> (SlideData, CaptchaAnswer) {
    let mut rng = thread_rng();

    // Image dimensions
    let width = 300u32;
    let height = 180u32;
    let piece_size = 50u32;

    // Random position for the piece
    let piece_x = rng.gen_range(80..width - piece_size - 20);
    let piece_y = rng.gen_range(20..height - piece_size - 20);

    // Generate background with gradient
    let mut bg = RgbaImage::new(width, height);
    let colors = [
        (66, 133, 244),   // Blue
        (52, 168, 83),    // Green
        (251, 188, 5),    // Yellow
        (234, 67, 53),    // Red
    ];
    let color = colors[rng.gen_range(0..colors.len())];

    for y in 0..height {
        for x in 0..width {
            let noise: i32 = rng.gen_range(-20..20);
            let r = (color.0 as i32 + noise).clamp(0, 255) as u8;
            let g = (color.1 as i32 + noise).clamp(0, 255) as u8;
            let b = (color.2 as i32 + noise).clamp(0, 255) as u8;
            bg.put_pixel(x, y, Rgba([r, g, b, 255]));
        }
    }

    // Add some random shapes for complexity
    add_random_shapes(&mut bg, &mut rng);

    // Create piece (extract from background)
    let mut piece = RgbaImage::new(piece_size, piece_size);
    for py in 0..piece_size {
        for px in 0..piece_size {
            let src_x = piece_x + px;
            let src_y = piece_y + py;
            if src_x < width && src_y < height {
                piece.put_pixel(px, py, *bg.get_pixel(src_x, src_y));
            }
        }
    }

    // Add shadow/outline to piece
    add_piece_outline(&mut piece);

    // Create hole in background
    for py in 0..piece_size {
        for px in 0..piece_size {
            let src_x = piece_x + px;
            let src_y = piece_y + py;
            if src_x < width && src_y < height {
                let pixel = bg.get_pixel_mut(src_x, src_y);
                pixel[0] = (pixel[0] as f32 * 0.5) as u8;
                pixel[1] = (pixel[1] as f32 * 0.5) as u8;
                pixel[2] = (pixel[2] as f32 * 0.5) as u8;
            }
        }
    }

    let data = SlideData {
        image: image_to_base64(&bg),
        thumb: image_to_base64(&piece),
        thumb_x: piece_x,
        thumb_y: piece_y,
        thumb_width: piece_size,
        thumb_height: piece_size,
    };

    let answer = CaptchaAnswer {
        captcha_type: CaptchaType::Slide,
        slide_x: Some(piece_x),
        click_dots: None,
        rotate_angle: None,
    };

    (data, answer)
}

/// Generate click captcha (simplified version)
pub fn generate_click_captcha() -> (ClickData, CaptchaAnswer) {
    let mut rng = thread_rng();

    let width = 300u32;
    let height = 200u32;

    // Generate background
    let mut bg = RgbaImage::new(width, height);
    let bg_color = (240u8, 240u8, 245u8);
    for pixel in bg.pixels_mut() {
        *pixel = Rgba([bg_color.0, bg_color.1, bg_color.2, 255]);
    }

    add_random_shapes(&mut bg, &mut rng);

    // Generate dots to click (3 dots)
    let mut dots = Vec::new();
    for _ in 0..3 {
        let x = rng.gen_range(30..width - 30);
        let y = rng.gen_range(30..height - 30);
        dots.push((x, y));

        // Draw dot marker
        draw_circle(&mut bg, x as i32, y as i32, 15, Rgba([255, 100, 100, 200]));
    }

    // Create hint image (smaller)
    let hint = RgbaImage::new(100, 40);

    let data = ClickData {
        image: image_to_base64(&bg),
        thumb: image_to_base64(&hint),
    };

    let answer = CaptchaAnswer {
        captcha_type: CaptchaType::Click,
        slide_x: None,
        click_dots: Some(dots),
        rotate_angle: None,
    };

    (data, answer)
}

/// Generate rotate captcha
pub fn generate_rotate_captcha() -> (RotateData, CaptchaAnswer) {
    let mut rng = thread_rng();

    let size = 200u32;
    let thumb_size = 150u32;

    // Random rotation angle (in degrees)
    let target_angle = rng.gen_range(30..330);

    // Generate circular image
    let mut img = RgbaImage::new(size, size);
    let center = size as i32 / 2;

    for y in 0..size {
        for x in 0..size {
            let dx = x as i32 - center;
            let dy = y as i32 - center;
            let dist = ((dx * dx + dy * dy) as f32).sqrt();

            if dist < center as f32 {
                let angle = (dy as f32).atan2(dx as f32);
                let hue = ((angle + std::f32::consts::PI) / (2.0 * std::f32::consts::PI) * 360.0) as u32;
                let (r, g, b) = hsv_to_rgb(hue, 0.7, 0.9);
                img.put_pixel(x, y, Rgba([r, g, b, 255]));
            } else {
                img.put_pixel(x, y, Rgba([0, 0, 0, 0]));
            }
        }
    }

    // Add direction indicator
    draw_line(&mut img, center, center, center + 60, center, Rgba([255, 255, 255, 255]));

    let data = RotateData {
        image: image_to_base64(&img),
        thumb: image_to_base64(&img),
        angle: target_angle,
        thumb_size,
    };

    let answer = CaptchaAnswer {
        captcha_type: CaptchaType::Rotate,
        slide_x: None,
        click_dots: None,
        rotate_angle: Some(target_angle),
    };

    (data, answer)
}

// Helper functions

fn image_to_base64(img: &RgbaImage) -> String {
    let mut buf = Cursor::new(Vec::new());
    img.write_to(&mut buf, image::ImageFormat::Png).unwrap();
    format!("data:image/png;base64,{}", BASE64.encode(buf.into_inner()))
}

fn add_random_shapes(img: &mut RgbaImage, rng: &mut impl Rng) {
    let (w, h) = img.dimensions();
    for _ in 0..5 {
        let x = rng.gen_range(0..w as i32);
        let y = rng.gen_range(0..h as i32);
        let r = rng.gen_range(10..30);
        let color = Rgba([
            rng.gen_range(100..200),
            rng.gen_range(100..200),
            rng.gen_range(100..200),
            100,
        ]);
        draw_circle(img, x, y, r, color);
    }
}

fn add_piece_outline(img: &mut RgbaImage) {
    let (w, h) = img.dimensions();
    for x in 0..w {
        img.put_pixel(x, 0, Rgba([50, 50, 50, 255]));
        img.put_pixel(x, h - 1, Rgba([50, 50, 50, 255]));
    }
    for y in 0..h {
        img.put_pixel(0, y, Rgba([50, 50, 50, 255]));
        img.put_pixel(w - 1, y, Rgba([50, 50, 50, 255]));
    }
}

fn draw_circle(img: &mut RgbaImage, cx: i32, cy: i32, radius: i32, color: Rgba<u8>) {
    let (w, h) = img.dimensions();
    for dy in -radius..=radius {
        for dx in -radius..=radius {
            if dx * dx + dy * dy <= radius * radius {
                let x = cx + dx;
                let y = cy + dy;
                if x >= 0 && x < w as i32 && y >= 0 && y < h as i32 {
                    let pixel = img.get_pixel_mut(x as u32, y as u32);
                    // Alpha blend
                    let alpha = color[3] as f32 / 255.0;
                    pixel[0] = ((1.0 - alpha) * pixel[0] as f32 + alpha * color[0] as f32) as u8;
                    pixel[1] = ((1.0 - alpha) * pixel[1] as f32 + alpha * color[1] as f32) as u8;
                    pixel[2] = ((1.0 - alpha) * pixel[2] as f32 + alpha * color[2] as f32) as u8;
                }
            }
        }
    }
}

fn draw_line(img: &mut RgbaImage, x0: i32, y0: i32, x1: i32, y1: i32, color: Rgba<u8>) {
    let (w, h) = img.dimensions();
    let dx = (x1 - x0).abs();
    let dy = (y1 - y0).abs();
    let sx = if x0 < x1 { 1 } else { -1 };
    let sy = if y0 < y1 { 1 } else { -1 };
    let mut err = dx - dy;
    let mut x = x0;
    let mut y = y0;

    loop {
        if x >= 0 && x < w as i32 && y >= 0 && y < h as i32 {
            img.put_pixel(x as u32, y as u32, color);
        }
        if x == x1 && y == y1 { break; }
        let e2 = 2 * err;
        if e2 > -dy { err -= dy; x += sx; }
        if e2 < dx { err += dx; y += sy; }
    }
}

fn hsv_to_rgb(h: u32, s: f32, v: f32) -> (u8, u8, u8) {
    let h = h % 360;
    let c = v * s;
    let x = c * (1.0 - ((h as f32 / 60.0) % 2.0 - 1.0).abs());
    let m = v - c;

    let (r, g, b) = match h {
        0..=59 => (c, x, 0.0),
        60..=119 => (x, c, 0.0),
        120..=179 => (0.0, c, x),
        180..=239 => (0.0, x, c),
        240..=299 => (x, 0.0, c),
        _ => (c, 0.0, x),
    };

    (((r + m) * 255.0) as u8, ((g + m) * 255.0) as u8, ((b + m) * 255.0) as u8)
}
