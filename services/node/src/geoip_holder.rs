use std::sync::Arc;
use parking_lot::RwLock;

use crate::geoip::GeoIpResolver;

pub struct GeoIpHolder {
	inner: Arc<RwLock<Option<Arc<GeoIpResolver>>>>,
}

impl GeoIpHolder {
	pub fn new(initial: Option<Arc<GeoIpResolver>>) -> Self {
		Self {
			inner: Arc::new(RwLock::new(initial)),
		}
	}

	pub fn get(&self) -> Option<Arc<GeoIpResolver>> {
		self.inner.read().clone()
	}

	pub fn set(&self, resolver: Option<Arc<GeoIpResolver>>) {
		*self.inner.write() = resolver;
	}
}

