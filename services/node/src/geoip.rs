use anyhow::{Context, Result};
use maxminddb::{geoip2, Reader};
use std::net::IpAddr;
use std::path::Path;

pub struct GeoIpResolver {
	reader: Reader<Vec<u8>>,
}

impl GeoIpResolver {
	pub fn from_path(path: &Path) -> Result<Self> {
		let reader = Reader::open_readfile(path)
			.with_context(|| format!("failed to open geoip db: {}", path.display()))?;
		Ok(Self { reader })
	}

	pub fn lookup(&self, ip: &str) -> Option<String> {
		let ip: IpAddr = ip.parse().ok()?;
		let result = self.reader.lookup(ip).ok()?;
		let city: geoip2::City = result.decode().ok()??;
		format_location(&city)
	}

	/// Returns only the ISO 3166-1 alpha-2 country code (e.g. "CN", "US").
	pub fn lookup_country(&self, ip: &str) -> Option<String> {
		let ip: IpAddr = ip.parse().ok()?;
		let result = self.reader.lookup(ip).ok()?;
		let city: geoip2::City = result.decode().ok()??;
		city.country.iso_code.map(|s| s.to_ascii_uppercase())
	}
}

fn format_location(city: &geoip2::City) -> Option<String> {
	let mut parts: Vec<String> = Vec::new();

	if let Some(code) = city.country.iso_code {
		parts.push(code.to_string());
	} else if let Some(name) = city.country.names.english {
		parts.push(name.to_string());
	}

	let region = city
		.subdivisions
		.first()
		.and_then(|sub| sub.names.english.or(sub.iso_code));
	if let Some(region) = region {
		if !region.is_empty() {
			parts.push(region.to_string());
		}
	}

	if let Some(city_name) = city.city.names.english {
		if !city_name.is_empty() {
			parts.push(city_name.to_string());
		}
	}

	if parts.is_empty() {
		None
	} else {
		Some(parts.join("/"))
	}
}
